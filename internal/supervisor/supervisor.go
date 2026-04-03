package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/swamp-dev/agentbox/internal/journal"
	"github.com/swamp-dev/agentbox/internal/metrics"
	"github.com/swamp-dev/agentbox/internal/ralph"
	"github.com/swamp-dev/agentbox/internal/review"
	"github.com/swamp-dev/agentbox/internal/store"
	"github.com/swamp-dev/agentbox/internal/taskdb"
	"github.com/swamp-dev/agentbox/internal/workflow"
)

// Supervisor orchestrates the full autonomous development lifecycle.
type Supervisor struct {
	cfg       *Config
	store     *store.Store
	sessionID int64
	workflow  *workflow.GitWorkflow
	taskDB    *taskdb.DB
	collector *metrics.Collector
	budget    *metrics.BudgetEnforcer
	journal   *journal.Journal
	reviewer  *review.Reviewer
	logger    *slog.Logger
}

// New creates a new Supervisor from configuration.
func New(cfg *Config, logger *slog.Logger) (*Supervisor, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Determine working directory.
	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = "."
	}

	// Ensure .agentbox directory exists.
	agentboxDir := filepath.Join(workDir, ".agentbox")
	if err := os.MkdirAll(agentboxDir, 0755); err != nil {
		return nil, fmt.Errorf("creating .agentbox directory: %w", err)
	}

	// Open SQLite store.
	dbPath := filepath.Join(agentboxDir, "agentbox.db")
	s, err := store.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening store: %w", err)
	}

	// Create session.
	cfgJSON, _ := json.Marshal(cfg)
	sessionID, err := s.CreateSession(cfg.RepoURL, cfg.BranchName, string(cfgJSON))
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("creating session: %w", err)
	}

	// Create git workflow.
	wf := workflow.NewGitWorkflow(cfg.RepoURL, workDir, logger)

	// Create metrics collector and budget enforcer.
	collector := metrics.NewCollector(s, sessionID)
	budget := metrics.NewBudgetEnforcer(cfg.Budget)

	// Create journal.
	j := journal.New(s, sessionID)

	tdb := taskdb.New()

	return &Supervisor{
		cfg:       cfg,
		store:     s,
		sessionID: sessionID,
		workflow:  wf,
		taskDB:    tdb,
		collector: collector,
		budget:    budget,
		journal:   j,
		logger:    logger,
	}, nil
}

// NewForResume creates a Supervisor that resumes an existing session.
// It loads the session's config from the store, restores task state, and
// skips the setup phase (worktree already exists on disk).
func NewForResume(sessionID int64, logger *slog.Logger) (*Supervisor, error) {
	// We need to find the store. Look in .agentbox relative to cwd first.
	workDir, _ := os.Getwd()
	agentboxDir := filepath.Join(workDir, ".agentbox")
	dbPath := filepath.Join(agentboxDir, "agentbox.db")

	s, err := store.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening store: %w", err)
	}

	return newForResumeWithStore(s, sessionID, workDir, logger)
}

// newForResumeWithStore is the internal constructor for resume, allowing
// tests to inject a store directly.
func newForResumeWithStore(s *store.Store, sessionID int64, workDir string, logger *slog.Logger) (*Supervisor, error) {
	sess, err := s.GetSession(sessionID)
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("loading session: %w", err)
	}

	if sess.Status != "interrupted" && sess.Status != "running" {
		s.Close()
		return nil, fmt.Errorf("session %d has status %q, not resumable (must be 'interrupted' or 'running')", sessionID, sess.Status)
	}

	// Restore config from session.
	cfg := DefaultConfig()
	if sess.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(sess.ConfigJSON), cfg); err != nil {
			s.Close()
			return nil, fmt.Errorf("parsing session config: %w", err)
		}
	}

	if workDir == "" && cfg.WorkDir != "" {
		workDir = cfg.WorkDir
	}
	if workDir == "" {
		workDir = "."
	}
	cfg.WorkDir = workDir

	// Create workflow and point it at the existing worktree.
	wf := workflow.NewGitWorkflow(cfg.RepoURL, workDir, logger)

	// Restore task state from store.
	tdb := taskdb.New()
	tasks, err := s.ListTasks(sessionID)
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("loading tasks: %w", err)
	}

	for _, st := range tasks {
		// Load dependencies.
		deps, _ := s.GetDependencies(st.ID)

		task := &taskdb.Task{
			ID:          st.ID,
			Title:       st.Title,
			Description: st.Description,
			Status:      taskdb.TaskStatus(st.Status),
			Priority:    st.Priority,
			Complexity:  st.Complexity,
			ParentID:    st.ParentID,
			MaxAttempts: st.MaxAttempts,
			DependsOn:   deps,
			CreatedAt:   st.CreatedAt,
			CompletedAt: st.CompletedAt,
		}

		// Restore attempts.
		attempts, _ := s.GetAttempts(st.ID)
		for _, a := range attempts {
			success := false
			if a.Success != nil {
				success = *a.Success
			}
			task.Attempts = append(task.Attempts, taskdb.Attempt{
				Number:    a.Number,
				AgentName: a.AgentName,
				Success:   success,
				ErrorMsg:  a.ErrorMsg,
				GitCommit: a.GitCommit,
				StartedAt: a.StartedAt,
			})
		}

		if err := tdb.Add(task); err != nil {
			s.Close()
			return nil, fmt.Errorf("restoring task %s: %w", st.ID, err)
		}
	}

	// Create metrics collector and budget enforcer.
	collector := metrics.NewCollector(s, sessionID)
	budget := metrics.NewBudgetEnforcer(cfg.Budget)
	j := journal.New(s, sessionID)

	// Mark session as running again.
	if err := s.UpdateSessionStatus(sessionID, "running"); err != nil {
		s.Close()
		return nil, fmt.Errorf("updating session status: %w", err)
	}

	return &Supervisor{
		cfg:       cfg,
		store:     s,
		sessionID: sessionID,
		workflow:  wf,
		taskDB:    tdb,
		collector: collector,
		budget:    budget,
		journal:   j,
		logger:    logger,
	}, nil
}

// Resume continues a previously interrupted session. It skips the setup
// phase and picks up the sprint loop from where it left off.
func (s *Supervisor) Resume(ctx context.Context) error {
	defer s.store.Close()

	// Determine where we left off.
	lastSprint, err := s.store.MaxSprintForSession(s.sessionID)
	if err != nil {
		return fmt.Errorf("determining last sprint: %w", err)
	}
	lastIter, err := s.store.MaxIterationForSession(s.sessionID)
	if err != nil {
		return fmt.Errorf("determining last iteration: %w", err)
	}

	startSprint := lastSprint + 1
	startIter := lastIter + 1

	total, completed, pending, failed, _ := s.taskDB.Stats()
	s.logger.Info("resuming session",
		"session_id", s.sessionID,
		"start_sprint", startSprint,
		"start_iteration", startIter,
		"tasks_total", total,
		"tasks_completed", completed,
		"tasks_pending", pending,
		"tasks_failed", failed,
	)

	// Resolve the worktree path. For resume, the worktree should already exist.
	// Try to locate it from the branch name.
	if s.cfg.BranchName != "" {
		repoDir := s.workflow.RepoDir()
		worktreeName := strings.ReplaceAll(s.cfg.BranchName, "/", "-")
		candidatePath := filepath.Join(filepath.Dir(repoDir), worktreeName)
		if fi, err := os.Stat(candidatePath); err == nil && fi.IsDir() {
			s.workflow.SetWorktreePath(candidatePath, s.cfg.BranchName)
		}
	}

	if s.cfg.JournalEnabled {
		_ = s.journal.Add(&store.JournalEntry{
			Kind:       string(journal.KindReflection),
			Sprint:     startSprint,
			Iteration:  startIter,
			Summary:    "Session resumed",
			Reflection: fmt.Sprintf("Resuming interrupted session. Tasks: %d total, %d completed, %d pending, %d failed.", total, completed, pending, failed),
			Confidence: 3,
			Momentum:   3,
		})
	}

	// Create agent runner (unless dry-run).
	var agentRunner AgentRunner
	var loop *ralph.Loop
	if !s.cfg.DryRun {
		worktreePath := s.workflow.WorktreePath()
		if worktreePath != "" {
			ralphCfg := s.cfg.ToRalphConfig()
			var err error
			loop, err = ralph.NewLoop(ralphCfg, worktreePath, s.logger)
			if err != nil {
				_ = s.store.UpdateSessionStatus(s.sessionID, "failed")
				return fmt.Errorf("creating ralph loop: %w", err)
			}
			defer func() {
				if closeErr := loop.Close(); closeErr != nil {
					s.logger.Warn("failed to close ralph loop", "error", closeErr)
				}
			}()
			agentRunner = NewRalphAgentRunner(loop)
		}
	}

	// Sprint loop — picks up where we left off.
	iteration := startIter
	for sprint := startSprint; sprint <= s.cfg.MaxSprints; sprint++ {
		select {
		case <-ctx.Done():
			_ = s.store.UpdateSessionStatus(s.sessionID, "interrupted")
			return ctx.Err()
		default:
		}

		if s.taskDB.IsComplete() {
			s.logger.Info("all tasks completed")
			break
		}

		runner := NewSprintRunner(
			s.cfg, s.store, s.sessionID,
			s.workflow, s.taskDB, s.collector, s.budget, s.journal, agentRunner, s.logger,
		)

		result, err := runner.RunSprint(ctx, sprint, iteration)
		if err != nil {
			s.logger.Error("sprint error", "sprint", sprint, "error", err)
			if result != nil && result.BudgetExceeded {
				s.logger.Warn("stopping: budget exceeded")
				break
			}
			continue
		}

		iteration = runner.CurrentIteration()

		if result.BudgetExceeded {
			s.logger.Warn("stopping: budget exceeded")
			break
		}
		if result.AbortedEarly {
			s.logger.Warn("sprint aborted", "reason", result.AbortReason)
		}

		if s.cfg.ReviewEnabled && s.cfg.ReviewAfter == "sprint" {
			s.runReviewGate(ctx)
		}
	}

	return s.finalize(ctx)
}

// Run executes the full supervisor lifecycle.
func (s *Supervisor) Run(ctx context.Context) error {
	defer s.store.Close()

	s.logger.Info("supervisor starting",
		"repo", s.cfg.RepoURL,
		"agent", s.cfg.Agent,
		"max_sprints", s.cfg.MaxSprints,
		"sprint_size", s.cfg.SprintSize,
	)

	// Phase 1: Setup.
	if err := s.setup(ctx); err != nil {
		if ctx.Err() != nil {
			_ = s.store.UpdateSessionStatus(s.sessionID, "interrupted")
		} else {
			_ = s.store.UpdateSessionStatus(s.sessionID, "failed")
		}
		return fmt.Errorf("setup: %w", err)
	}

	// Create agent runner (unless dry-run).
	var agentRunner AgentRunner
	var loop *ralph.Loop
	if !s.cfg.DryRun {
		ralphCfg := s.cfg.ToRalphConfig()
		worktreePath := s.workflow.WorktreePath()
		if worktreePath == "" {
			_ = s.store.UpdateSessionStatus(s.sessionID, "failed")
			return fmt.Errorf("creating agent runner: worktree path not set")
		}
		var err error
		loop, err = ralph.NewLoop(ralphCfg, worktreePath, s.logger)
		if err != nil {
			_ = s.store.UpdateSessionStatus(s.sessionID, "failed")
			return fmt.Errorf("creating ralph loop (image=%s, prd=%s): %w", ralphCfg.Docker.Image, ralphCfg.Ralph.PRDFile, err)
		}
		defer func() {
			if closeErr := loop.Close(); closeErr != nil {
				s.logger.Warn("failed to close ralph loop", "error", closeErr)
			}
		}()
		agentRunner = NewRalphAgentRunner(loop)
	}

	// Phase 2: Sprint loop.
	iteration := 1
	for sprint := 1; sprint <= s.cfg.MaxSprints; sprint++ {
		select {
		case <-ctx.Done():
			_ = s.store.UpdateSessionStatus(s.sessionID, "interrupted")
			return ctx.Err()
		default:
		}

		if s.taskDB.IsComplete() {
			s.logger.Info("all tasks completed")
			break
		}

		runner := NewSprintRunner(
			s.cfg, s.store, s.sessionID,
			s.workflow, s.taskDB, s.collector, s.budget, s.journal, agentRunner, s.logger,
		)

		result, err := runner.RunSprint(ctx, sprint, iteration)
		if err != nil {
			s.logger.Error("sprint error", "sprint", sprint, "error", err)
			if result != nil && result.BudgetExceeded {
				s.logger.Warn("stopping: budget exceeded")
				break
			}
			continue
		}

		iteration = runner.CurrentIteration()

		// Check if adaptive controller recommended an agent switch.
		if recommended, newAgent := runner.SwitchRecommended(); recommended && !s.cfg.DryRun {
			s.logger.Info("rebuilding agent runner for next sprint", "new_agent", newAgent)
			s.cfg.Agent = newAgent

			// Close the old loop and build a new one with the updated agent.
			if closeErr := loop.Close(); closeErr != nil {
				s.logger.Warn("failed to close old ralph loop", "error", closeErr)
			}
			ralphCfg := s.cfg.ToRalphConfig()
			newLoop, err := ralph.NewLoop(ralphCfg, s.workflow.WorktreePath(), s.logger)
			if err != nil {
				s.logger.Error("failed to create ralph loop with new agent", "agent", newAgent, "error", err)
				// Continue with old runner rather than failing the session.
			} else {
				loop = newLoop
				agentRunner = NewRalphAgentRunner(loop)
			}
		}

		if result.BudgetExceeded {
			s.logger.Warn("stopping: budget exceeded")
			break
		}
		if result.AbortedEarly {
			s.logger.Warn("sprint aborted", "reason", result.AbortReason)
		}

		// Phase 3: Review gate (if configured for after each sprint).
		if s.cfg.ReviewEnabled && s.cfg.ReviewAfter == "sprint" {
			s.runReviewGate(ctx)
		}
	}

	// Phase 4: Finalize.
	return s.finalize(ctx)
}

// setup performs Phase 1: clone repo, create worktree, import tasks.
func (s *Supervisor) setup(ctx context.Context) error {
	s.logger.Info("phase 1: setup")

	// Clone or open repo.
	if err := s.workflow.CloneOrOpen(ctx); err != nil {
		return fmt.Errorf("opening repository: %w", err)
	}

	// Create worktree.
	if err := s.workflow.CreateWorktree(ctx, s.cfg.BranchName); err != nil {
		return fmt.Errorf("creating worktree: %w", err)
	}

	// Import PRD into task database.
	if err := s.importPRD(); err != nil {
		return fmt.Errorf("importing PRD: %w", err)
	}

	// Ensure .agentbox directory in worktree.
	agentboxDir := filepath.Join(s.workflow.WorktreePath(), ".agentbox")
	if err := os.MkdirAll(agentboxDir, 0755); err != nil {
		return fmt.Errorf("creating worktree .agentbox directory: %w", err)
	}

	// Write initial journal entry.
	if s.cfg.JournalEnabled {
		total, _, _, _, _ := s.taskDB.Stats()
		_ = s.journal.Add(&store.JournalEntry{
			Kind:       string(journal.KindReflection),
			Sprint:     0,
			Iteration:  0,
			Summary:    "Session started",
			Reflection: fmt.Sprintf("Starting new sprint session with %d tasks. Agent: %s. Let's see how this goes.", total, s.cfg.Agent),
			Confidence: 3,
			Momentum:   3,
		})
	}

	return nil
}

// importPRD loads the PRD file and imports tasks into the task database and store.
func (s *Supervisor) importPRD() error {
	prdPath := s.cfg.PRDFile
	if !filepath.IsAbs(prdPath) {
		prdPath = filepath.Join(s.workflow.WorktreePath(), prdPath)
	}

	prd, err := ralph.LoadPRD(prdPath)
	if err != nil {
		return err
	}

	exportedTasks := prd.ExportTasks()
	for _, t := range exportedTasks {
		task := &taskdb.Task{
			ID:          t.ID,
			Title:       t.Title,
			Description: t.Description,
			Status:      taskdb.TaskStatus(t.Status),
			Priority:    t.Priority,
			DependsOn:   t.DependsOn,
			MaxAttempts: 3,
			Complexity:  3,
		}
		if task.Status == "" {
			task.Status = taskdb.StatusPending
		}
		if err := s.taskDB.Add(task); err != nil {
			return fmt.Errorf("adding task %s to taskDB: %w", t.ID, err)
		}

		// Also insert into store.
		if err := s.store.InsertTask(&store.Task{
			ID:          t.ID,
			SessionID:   s.sessionID,
			Title:       t.Title,
			Description: t.Description,
			Status:      string(task.Status),
			Priority:    t.Priority,
			MaxAttempts: 3,
			Complexity:  3,
		}); err != nil {
			return fmt.Errorf("inserting task %s into store: %w", t.ID, err)
		}

		// Add dependencies.
		for _, dep := range t.DependsOn {
			if err := s.store.AddDependency(t.ID, dep); err != nil {
				return fmt.Errorf("adding dependency %s -> %s: %w", t.ID, dep, err)
			}
		}
	}

	total, _, _, _, _ := s.taskDB.Stats()
	s.logger.Info("imported tasks", "count", total, "prd", prd.Name)
	return nil
}

// runReviewGate executes the code review process.
func (s *Supervisor) runReviewGate(ctx context.Context) {
	if s.reviewer == nil {
		s.logger.Debug("skipping review: no reviewer configured")
		return
	}

	s.logger.Info("running review gate")

	// Get diff.
	diff, err := s.workflow.Diff(ctx, "origin/main")
	if err != nil {
		s.logger.Warn("could not get diff for review", "error", err)
		return
	}

	changedFiles, _ := s.workflow.DiffFiles(ctx, "origin/main")

	metricsSummary, _ := s.collector.Summary()

	for round := 1; round <= s.cfg.MaxReviewRounds; round++ {
		result, err := s.reviewer.Review(ctx, s.workflow.WorktreePath(), diff, changedFiles, metricsSummary)
		if err != nil {
			s.logger.Warn("review failed", "round", round, "error", err)
			break
		}

		// Save review result.
		findingsJSON, _ := json.Marshal(result.Findings)
		if err := s.store.SaveReviewResult(&store.ReviewResult{
			SessionID:    s.sessionID,
			Sprint:       0, // Will be updated when we know the sprint number.
			ReviewAgent:  result.ReviewAgent,
			FindingsJSON: string(findingsJSON),
			Summary:      result.Summary,
			Approved:     result.Approved,
		}); err != nil {
			s.logger.Warn("could not save review result", "error", err)
		}

		// Write journal entry.
		if s.cfg.JournalEnabled {
			counts := result.CountBySeverity()
			_ = s.journal.Add(&store.JournalEntry{
				Kind:      string(journal.KindReviewReceived),
				Iteration: 0,
				Summary:   fmt.Sprintf("Code review round %d: %v", round, counts),
				Reflection: fmt.Sprintf("Review by %s: %s. Approved: %v. Findings: critical=%d, significant=%d, minor=%d, nit=%d",
					result.ReviewAgent, result.Summary, result.Approved,
					counts[review.SeverityCritical], counts[review.SeveritySignificant],
					counts[review.SeverityMinor], counts[review.SeverityNit]),
			})
		}

		if result.Approved {
			s.logger.Info("review approved", "round", round)
			break
		}

		if round >= s.cfg.MaxReviewRounds {
			s.logger.Warn("max review rounds reached without approval")
			break
		}

		// Feed blockers back as tasks for the next sprint.
		for _, finding := range result.BlockerFindings() {
			fixTask := &taskdb.Task{
				ID:          fmt.Sprintf("review-fix-%d-%d", round, time.Now().UnixNano()%10000),
				Title:       fmt.Sprintf("Fix review finding: %s", finding.Description),
				Description: fmt.Sprintf("[%s] %s\nFile: %s\nSuggestion: %s", finding.Severity, finding.Description, finding.File, finding.Suggestion),
				Status:      taskdb.StatusPending,
				Priority:    0, // Highest priority.
				MaxAttempts: 2,
			}
			_ = s.taskDB.Add(fixTask)
			_ = s.store.InsertTask(&store.Task{
				ID:          fixTask.ID,
				SessionID:   s.sessionID,
				Title:       fixTask.Title,
				Description: fixTask.Description,
				Status:      "pending",
				Priority:    0,
				MaxAttempts: 2,
			})
		}
	}
}

// finalize performs Phase 4: final tests, PR creation, wrap-up.
func (s *Supervisor) finalize(ctx context.Context) error {
	s.logger.Info("phase 4: finalize")

	// Final review if enabled and not done after last sprint.
	if s.cfg.ReviewEnabled && s.cfg.ReviewAfter == "pr" {
		s.runReviewGate(ctx)
	}

	// Write final journal entry.
	if s.cfg.JournalEnabled {
		total, completed, pending, failed, deferred := s.taskDB.Stats()
		usage, _ := s.collector.TotalUsage()
		_ = s.journal.Add(&store.JournalEntry{
			Kind:      string(journal.KindFinalWrapUp),
			Sprint:    0,
			Iteration: 0,
			Summary:   "Session complete",
			Reflection: fmt.Sprintf(
				"Session finished. Tasks: %d total, %d completed, %d pending, %d failed, %d deferred. "+
					"Tokens: %d. Container time: %dms.",
				total, completed, pending, failed, deferred,
				usage.EstimatedTokens, usage.ContainerTimeMs,
			),
		})
	}

	// Generate PR body.
	prBody, err := s.generatePRBody()
	if err != nil {
		s.logger.Warn("could not generate PR body", "error", err)
		prBody = "Automated PR by agentbox"
	}

	// Open PR.
	total, completed, _, _, _ := s.taskDB.Stats()
	prTitle := fmt.Sprintf("agentbox: %d/%d tasks completed", completed, total)
	prURL, err := s.workflow.OpenPR(ctx, prTitle, prBody)
	if err != nil {
		s.logger.Warn("could not create PR", "error", err)
	} else {
		s.logger.Info("pull request created", "url", prURL)
	}

	// Export journal.
	md, err := s.journal.ExportMarkdown()
	if err == nil && md != "" {
		journalPath := filepath.Join(s.workflow.WorktreePath(), ".agentbox", "journal.md")
		if writeErr := os.WriteFile(journalPath, []byte(md), 0644); writeErr != nil {
			s.logger.Warn("could not write journal", "error", writeErr)
		}
	}

	_ = s.store.UpdateSessionStatus(s.sessionID, "completed")
	return nil
}

// generatePRBody creates the PR description from task DB and journal.
func (s *Supervisor) generatePRBody() (string, error) {
	total, completed, pending, failed, deferred := s.taskDB.Stats()

	body := fmt.Sprintf("## Summary\n\n"+
		"- **%d/%d** tasks completed\n"+
		"- %d pending, %d failed, %d deferred\n\n", completed, total, pending, failed, deferred)

	// Completed tasks.
	body += "## Completed Tasks\n\n"
	for _, task := range s.taskDB.CompletedTasks() {
		body += fmt.Sprintf("- [x] %s: %s\n", task.ID, task.Title)
	}
	body += "\n"

	// Failed/deferred tasks.
	unresolvedTasks := s.taskDB.TasksByStatus(taskdb.StatusFailed, taskdb.StatusDeferred)
	if len(unresolvedTasks) > 0 {
		body += "## Unresolved\n\n"
		for _, task := range unresolvedTasks {
			body += fmt.Sprintf("- [ ] %s: %s (%s)\n", task.ID, task.Title, task.Status)
		}
		body += "\n"
	}

	// Metrics.
	metricsSummary, _ := s.collector.Summary()
	if metricsSummary != "" {
		body += "## Metrics\n\n"
		body += metricsSummary + "\n\n"
	}

	body += "---\n\n🤖 Generated by [agentbox](https://github.com/swamp-dev/agentbox)\n"

	return body, nil
}

// FindResumableSession opens the store at the given workDir and returns
// the latest resumable session. The caller is responsible for the returned
// session's data only; the store is closed before returning.
func FindResumableSession(workDir string) (*store.Session, error) {
	dbPath := filepath.Join(workDir, ".agentbox", "agentbox.db")
	s, err := store.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening store: %w", err)
	}
	defer s.Close()

	sess, err := s.LatestResumableSession()
	if err != nil {
		return nil, err
	}
	// Return a copy so the store can be safely closed.
	copy := *sess
	return &copy, nil
}

// SessionID returns the current session ID.
func (s *Supervisor) SessionID() int64 {
	return s.sessionID
}

// Store returns the underlying store for external access.
func (s *Supervisor) Store() *store.Store {
	return s.store
}
