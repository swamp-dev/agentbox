package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/swamp-dev/agentbox/internal/journal"
	"github.com/swamp-dev/agentbox/internal/metrics"
	"github.com/swamp-dev/agentbox/internal/ralph"
	"github.com/swamp-dev/agentbox/internal/retro"
	"github.com/swamp-dev/agentbox/internal/store"
	"github.com/swamp-dev/agentbox/internal/taskdb"
	"github.com/swamp-dev/agentbox/internal/workflow"
)

// SprintRunner executes a sprint: N iterations + retro.
type SprintRunner struct {
	cfg        *Config
	store      *store.Store
	sessionID  int64
	workflow   *workflow.GitWorkflow
	taskDB     *taskdb.DB
	collector  *metrics.Collector
	budget     *metrics.BudgetEnforcer
	journal    *journal.Journal
	ctxBuilder *ContextBuilder
	adaptive   *AdaptiveController
	runner     AgentRunner
	logger     *slog.Logger

	sprintNum   int
	iteration   int
	consecutiveFails int
}

// SprintResult captures the outcome of a sprint.
type SprintResult struct {
	SprintNumber    int
	TasksAttempted  int
	TasksCompleted  int
	TasksFailed     int
	BudgetExceeded  bool
	AbortedEarly    bool
	AbortReason     string
}

// NewSprintRunner creates a new sprint runner.
// If runner is nil, a NoopAgentRunner is used.
func NewSprintRunner(
	cfg *Config,
	s *store.Store,
	sessionID int64,
	wf *workflow.GitWorkflow,
	tdb *taskdb.DB,
	collector *metrics.Collector,
	budget *metrics.BudgetEnforcer,
	j *journal.Journal,
	runner AgentRunner,
	logger *slog.Logger,
) *SprintRunner {
	if runner == nil {
		runner = &NoopAgentRunner{}
	}
	return &SprintRunner{
		cfg:        cfg,
		store:      s,
		sessionID:  sessionID,
		workflow:   wf,
		taskDB:     tdb,
		collector:  collector,
		budget:     budget,
		journal:    j,
		ctxBuilder: NewContextBuilder(s, sessionID),
		adaptive:   NewAdaptiveController(s, sessionID, logger),
		runner:     runner,
		logger:     logger,
	}
}

// RunSprint executes a single sprint of N iterations.
func (sr *SprintRunner) RunSprint(ctx context.Context, sprintNum, startIter int) (*SprintResult, error) {
	sr.sprintNum = sprintNum
	sr.iteration = startIter
	sr.consecutiveFails = 0

	result := &SprintResult{SprintNumber: sprintNum}
	sprintStart := time.Now()

	sr.logger.Info("starting sprint",
		"sprint", sprintNum,
		"start_iteration", startIter,
		"sprint_size", sr.cfg.SprintSize,
	)

	for i := 0; i < sr.cfg.SprintSize; i++ {
		select {
		case <-ctx.Done():
			result.AbortedEarly = true
			result.AbortReason = "context cancelled"
			return result, ctx.Err()
		default:
		}

		// Check budget.
		usage, _ := sr.collector.TotalUsage()
		budgetStatus := sr.budget.Check(usage.EstimatedTokens, sr.iteration)
		if budgetStatus.Exceeded {
			result.BudgetExceeded = true
			result.AbortedEarly = true
			result.AbortReason = budgetStatus.Reason
			sr.logger.Warn("budget exceeded", "reason", budgetStatus.Reason)
			break
		}
		if budgetStatus.Warning {
			sr.logger.Warn("budget warning", "reason", budgetStatus.Reason)
		}

		// Check consecutive failures.
		if sr.consecutiveFails >= sr.cfg.MaxConsecutiveFails {
			result.AbortedEarly = true
			result.AbortReason = fmt.Sprintf("%d consecutive failures", sr.consecutiveFails)
			sr.logger.Warn("aborting sprint early", "consecutive_fails", sr.consecutiveFails)
			break
		}

		// Get next task.
		task := sr.taskDB.NextTask()
		if task == nil {
			sr.logger.Info("no more tasks available")
			break
		}

		// Run the iteration.
		success := sr.runIteration(ctx, task)
		result.TasksAttempted++
		if success {
			result.TasksCompleted++
			sr.consecutiveFails = 0
		} else {
			result.TasksFailed++
			sr.consecutiveFails++
		}

		sr.iteration++
	}

	// Run retrospective.
	analyzer := retro.NewAnalyzer(sr.store, sr.sessionID)
	report, err := analyzer.Analyze(sprintNum, startIter, sr.iteration)
	if err == nil {
		report.Duration = time.Since(sprintStart)
		analyzer.SaveReport(report)

		// Apply recommendations.
		if len(report.Recommendations) > 0 {
			actions := sr.adaptive.Apply(report.Recommendations)
			for _, a := range actions {
				sr.logger.Info("retro action", "action", a)
			}
		}

		// Write sprint retro journal entry.
		if sr.cfg.JournalEnabled {
			sr.writeSprintRetroEntry(report)
		}
	}

	return result, nil
}

// runIteration executes a single task iteration.
func (sr *SprintRunner) runIteration(ctx context.Context, task *taskdb.Task) bool {
	sr.logger.Info("starting iteration",
		"iteration", sr.iteration,
		"task", task.ID,
		"title", task.Title,
		"attempt", len(task.Attempts)+1,
	)

	iterStart := time.Now()

	// Write journal entry for task start.
	if sr.cfg.JournalEnabled {
		sr.journal.Add(&store.JournalEntry{
			Kind:      string(journal.KindTaskStart),
			TaskID:    task.ID,
			Sprint:    sr.sprintNum,
			Iteration: sr.iteration,
			Summary:   fmt.Sprintf("Starting: %s", task.Title),
			Reflection: fmt.Sprintf("Beginning work on %s (attempt %d of %d)",
				task.Title, len(task.Attempts)+1, task.MaxAttempts),
		})
	}

	// Build enriched prompt.
	prompt := sr.ctxBuilder.BuildPrompt(task, sr.cfg.RepoURL)

	// Record attempt start.
	attemptNum := len(task.Attempts) + 1
	attempt := &store.Attempt{
		TaskID:    task.ID,
		SessionID: sr.sessionID,
		Number:    attemptNum,
		AgentName: sr.cfg.Agent,
		StartedAt: iterStart,
	}

	// Get commit SHA before running agent (for potential rollback).
	beforeSHA, _ := sr.workflow.CurrentCommit(ctx)

	// Record the attempt in the store.
	attemptID, err := sr.store.RecordAttempt(attempt)
	if err != nil {
		sr.logger.Error("failed to record attempt", "error", err)
		return false
	}

	// Execute the task via the agent runner.
	ralphTask := &ralph.Task{
		ID:          task.ID,
		Title:       task.Title,
		Description: task.Description,
	}
	agentResult := sr.runner.RunTask(ctx, ralphTask, prompt)
	success := agentResult.Success

	duration := time.Since(iterStart)

	// Record resource usage.
	sr.collector.RecordUsage(&store.ResourceUsage{
		Iteration:       sr.iteration,
		TaskID:          task.ID,
		AgentName:       sr.cfg.Agent,
		ContainerTimeMs: int(duration.Milliseconds()),
	})

	// Update attempt record.
	attempt.Success = &success
	attempt.DurationMs = int(duration.Milliseconds())
	attempt.CompletedAt = timePtr(time.Now())
	if agentResult.Error != "" {
		attempt.ErrorMsg = agentResult.Error
	}

	// Save transcript.
	transcript := agentResult.Output
	if transcript == "" {
		transcript = fmt.Sprintf("Prompt sent for task %s (iteration %d). Error: %s", task.ID, sr.iteration, agentResult.Error)
	}
	sr.store.SaveTranscript(attemptID, transcript)

	// Record attempt on the taskdb task.
	task.Attempts = append(task.Attempts, taskdb.Attempt{
		Number:    attemptNum,
		AgentName: sr.cfg.Agent,
		Success:   success,
		ErrorMsg:  agentResult.Error,
		StartedAt: iterStart,
		GitCommit: beforeSHA,
	})

	// Auto-commit on success.
	if success && sr.cfg.AutoCommit {
		msg := fmt.Sprintf("feat(%s): %s", task.ID, task.Title)
		if err := sr.workflow.Commit(ctx, msg, nil); err != nil {
			sr.logger.Warn("commit failed", "error", err)
		}
	}

	// Update task status.
	if success {
		task.Status = taskdb.StatusCompleted
		now := time.Now()
		task.CompletedAt = &now
		sr.store.UpdateTaskStatus(task.ID, "completed")
	}

	// Write journal entry for result.
	if sr.cfg.JournalEnabled {
		kind := journal.KindTaskComplete
		if !success {
			kind = journal.KindTaskFailed
		}
		sr.journal.Add(&store.JournalEntry{
			Kind:       string(kind),
			TaskID:     task.ID,
			Sprint:     sr.sprintNum,
			Iteration:  sr.iteration,
			Summary:    fmt.Sprintf("%s: %s", task.Status, task.Title),
			Reflection: fmt.Sprintf("Attempt %d on %s completed. Success: %v", attemptNum, task.Title, success),
			DurationMs: int(duration.Milliseconds()),
		})
	}

	return success
}

// writeSprintRetroEntry writes a journal entry summarizing the sprint retro.
func (sr *SprintRunner) writeSprintRetroEntry(report *retro.SprintReport) {
	patternsDesc := ""
	for _, p := range report.Patterns {
		patternsDesc += fmt.Sprintf("- [%s] %s\n", p.Type, p.Description)
	}

	recsDesc := ""
	for _, r := range report.Recommendations {
		recsDesc += fmt.Sprintf("- [%s] %s\n", r.Action, r.Description)
	}

	reflection := fmt.Sprintf(
		"Sprint %d completed. Velocity: %.1f%% (%d/%d tasks). Quality: %s. Pass rate: %.1f%%.\n\n",
		report.SprintNumber, report.Velocity*100,
		report.TasksCompleted, report.TasksAttempted,
		report.QualityTrend, report.TestPassRate*100,
	)
	if patternsDesc != "" {
		reflection += "Patterns detected:\n" + patternsDesc + "\n"
	}
	if recsDesc != "" {
		reflection += "Recommendations:\n" + recsDesc
	}

	sr.journal.Add(&store.JournalEntry{
		Kind:       string(journal.KindSprintRetro),
		Sprint:     sr.sprintNum,
		Iteration:  sr.iteration,
		Summary:    fmt.Sprintf("Sprint %d Retrospective", report.SprintNumber),
		Reflection: reflection,
	})
}

// CurrentIteration returns the current iteration count.
func (sr *SprintRunner) CurrentIteration() int {
	return sr.iteration
}

func timePtr(t time.Time) *time.Time {
	return &t
}

// marshalJSON is a helper that ignores errors.
func marshalJSON(v interface{}) string {
	data, _ := json.Marshal(v)
	return string(data)
}
