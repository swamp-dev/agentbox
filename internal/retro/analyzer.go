// Package retro provides sprint retrospective analysis and pattern detection.
package retro

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/swamp-dev/agentbox/internal/store"
)

// PatternType categorizes detected patterns.
type PatternType string

const (
	PatternRepeatedFailure    PatternType = "repeated_failure"
	PatternSameTestFailing    PatternType = "same_test_failing"
	PatternQualityDegradation PatternType = "quality_degradation"
	PatternStuck              PatternType = "stuck"
	PatternHighVelocity       PatternType = "high_velocity"
)

// Pattern represents a detected pattern in the sprint data.
type Pattern struct {
	Type        PatternType `json:"type"`
	Description string      `json:"description"`
	TaskIDs     []string    `json:"task_ids,omitempty"`
	Tests       []string    `json:"tests,omitempty"`
	Severity    string      `json:"severity"` // high, medium, low
}

// RecommendationType categorizes adaptive actions.
type RecommendationType string

const (
	RecReorderTasks   RecommendationType = "reorder_tasks"
	RecSplitTask      RecommendationType = "split_task"
	RecSwitchAgent    RecommendationType = "switch_agent"
	RecRollback       RecommendationType = "rollback"
	RecUpdateContext  RecommendationType = "update_context"
	RecEscalate       RecommendationType = "escalate"
	RecSkipTask       RecommendationType = "skip_task"
	RecDeferTask      RecommendationType = "defer_task"
)

// Recommendation is a suggested action based on detected patterns.
type Recommendation struct {
	Action      RecommendationType `json:"action"`
	TaskID      string             `json:"task_id,omitempty"`
	Description string             `json:"description"`
	Priority    int                `json:"priority"` // 1=highest
}

// SprintReport summarizes a sprint's performance.
type SprintReport struct {
	SprintNumber    int              `json:"sprint_number"`
	StartIteration  int              `json:"start_iteration"`
	EndIteration    int              `json:"end_iteration"`
	TasksAttempted  int              `json:"tasks_attempted"`
	TasksCompleted  int              `json:"tasks_completed"`
	TasksFailed     int              `json:"tasks_failed"`
	Velocity        float64          `json:"velocity"` // completed / attempted
	QualityTrend    string           `json:"quality_trend"`
	TestPassRate    float64          `json:"test_pass_rate"`
	Patterns        []Pattern        `json:"patterns"`
	Recommendations []Recommendation `json:"recommendations"`
	TotalTokens     int              `json:"total_tokens"`
	Duration        time.Duration    `json:"duration"`
}

// Analyzer performs sprint retrospective analysis.
type Analyzer struct {
	store     *store.Store
	sessionID int64
}

// NewAnalyzer creates a new retrospective analyzer.
func NewAnalyzer(s *store.Store, sessionID int64) *Analyzer {
	return &Analyzer{store: s, sessionID: sessionID}
}

// Analyze runs retrospective analysis on the given iteration range.
func (a *Analyzer) Analyze(sprintNum, startIter, endIter int) (*SprintReport, error) {
	report := &SprintReport{
		SprintNumber:   sprintNum,
		StartIteration: startIter,
		EndIteration:   endIter,
	}

	// Count task outcomes from attempts in this range.
	tasks, err := a.store.ListTasks(a.sessionID)
	if err != nil {
		return nil, fmt.Errorf("listing tasks: %w", err)
	}

	taskAttempted := make(map[string]bool)
	taskCompleted := make(map[string]bool)
	taskFailed := make(map[string]bool)

	for _, task := range tasks {
		attempts, err := a.store.GetAttempts(task.ID)
		if err != nil {
			continue
		}
		for _, att := range attempts {
			if att.Number >= startIter && att.Number <= endIter {
				taskAttempted[task.ID] = true
				if att.Success != nil && *att.Success {
					taskCompleted[task.ID] = true
				} else {
					taskFailed[task.ID] = true
				}
			}
		}
	}

	report.TasksAttempted = len(taskAttempted)
	report.TasksCompleted = len(taskCompleted)
	report.TasksFailed = len(taskFailed)
	if report.TasksAttempted > 0 {
		report.Velocity = float64(report.TasksCompleted) / float64(report.TasksAttempted)
	}

	// Quality metrics.
	trend, _ := a.store.QualityTrend(a.sessionID, endIter-startIter+1)
	report.QualityTrend = trend

	passRate, _ := a.store.TestPassRate(a.sessionID, endIter-startIter+1)
	report.TestPassRate = passRate

	// Usage.
	usage, _ := a.store.TotalUsage(a.sessionID)
	if usage != nil {
		report.TotalTokens = usage.EstimatedTokens
	}

	// Detect patterns.
	report.Patterns = a.detectPatterns(tasks, startIter, endIter)

	// Generate recommendations.
	report.Recommendations = a.generateRecommendations(report.Patterns, tasks)

	return report, nil
}

// detectPatterns identifies recurring issues in the sprint data.
func (a *Analyzer) detectPatterns(tasks []*store.Task, startIter, endIter int) []Pattern {
	var patterns []Pattern

	// Check for repeated failures on same task.
	for _, task := range tasks {
		attempts, err := a.store.GetAttempts(task.ID)
		if err != nil {
			continue
		}
		failCount := 0
		for _, att := range attempts {
			if att.Success != nil && !*att.Success {
				failCount++
			}
		}
		if failCount >= 2 {
			patterns = append(patterns, Pattern{
				Type:        PatternRepeatedFailure,
				Description: fmt.Sprintf("Task %q has failed %d times", task.Title, failCount),
				TaskIDs:     []string{task.ID},
				Severity:    severityFromFailCount(failCount),
			})
		}
	}

	// Check for same tests failing repeatedly.
	failingTests, _ := a.store.FailingTestTrend(a.sessionID, endIter-startIter+1)
	for testName, count := range failingTests {
		if count >= 3 {
			patterns = append(patterns, Pattern{
				Type:        PatternSameTestFailing,
				Description: fmt.Sprintf("Test %q has failed in %d snapshots", testName, count),
				Tests:       []string{testName},
				Severity:    "high",
			})
		}
	}

	// Check for quality degradation.
	trend, _ := a.store.QualityTrend(a.sessionID, endIter-startIter+1)
	if trend == "degrading" {
		patterns = append(patterns, Pattern{
			Type:        PatternQualityDegradation,
			Description: "Overall quality is degrading across iterations",
			Severity:    "high",
		})
	}

	// Check for stuck state (multiple consecutive failures).
	if a.isStuck(tasks) {
		patterns = append(patterns, Pattern{
			Type:        PatternStuck,
			Description: "Multiple consecutive iterations have failed",
			Severity:    "high",
		})
	}

	return patterns
}

// isStuck checks if the most recent attempts across all tasks are all failures.
func (a *Analyzer) isStuck(tasks []*store.Task) bool {
	// Collect all attempts and sort by start time (newest first).
	type attemptInfo struct {
		startedAt time.Time
		success   *bool
	}
	var allAttempts []attemptInfo
	for _, task := range tasks {
		attempts, _ := a.store.GetAttempts(task.ID)
		for _, att := range attempts {
			allAttempts = append(allAttempts, attemptInfo{
				startedAt: att.StartedAt,
				success:   att.Success,
			})
		}
	}

	// Sort newest first.
	sort.Slice(allAttempts, func(i, j int) bool {
		return allAttempts[i].startedAt.After(allAttempts[j].startedAt)
	})

	// Check last N attempts for consecutive failures.
	consecutiveFails := 0
	for _, att := range allAttempts {
		if att.success != nil && !*att.success {
			consecutiveFails++
		} else {
			break
		}
	}
	return consecutiveFails >= 3
}

// generateRecommendations produces actionable suggestions from patterns.
func (a *Analyzer) generateRecommendations(patterns []Pattern, tasks []*store.Task) []Recommendation {
	var recs []Recommendation

	for _, p := range patterns {
		switch p.Type {
		case PatternRepeatedFailure:
			if p.Severity == "high" {
				for _, taskID := range p.TaskIDs {
					recs = append(recs, Recommendation{
						Action:      RecDeferTask,
						TaskID:      taskID,
						Description: fmt.Sprintf("Defer task after repeated failures: %s", p.Description),
						Priority:    1,
					})
				}
			} else {
				for _, taskID := range p.TaskIDs {
					recs = append(recs, Recommendation{
						Action:      RecUpdateContext,
						TaskID:      taskID,
						Description: fmt.Sprintf("Add failure context to avoid repeating: %s", p.Description),
						Priority:    2,
					})
				}
			}

		case PatternSameTestFailing:
			recs = append(recs, Recommendation{
				Action:      RecUpdateContext,
				Description: fmt.Sprintf("Focus on fixing persistently failing tests: %v", p.Tests),
				Priority:    1,
			})

		case PatternQualityDegradation:
			recs = append(recs, Recommendation{
				Action:      RecRollback,
				Description: "Quality is degrading — consider rolling back to last known-good state",
				Priority:    1,
			})

		case PatternStuck:
			recs = append(recs, Recommendation{
				Action:      RecSwitchAgent,
				Description: "Multiple consecutive failures — try switching to fallback agent",
				Priority:    1,
			})
			recs = append(recs, Recommendation{
				Action:      RecEscalate,
				Description: "System appears stuck — escalate for human review",
				Priority:    2,
			})
		}
	}

	return recs
}

// SaveReport persists a sprint report to the store.
func (a *Analyzer) SaveReport(report *SprintReport) error {
	patternsJSON, _ := json.Marshal(report.Patterns)
	recsJSON, _ := json.Marshal(report.Recommendations)

	return a.store.SaveSprintReport(&store.SprintReport{
		SessionID:           a.sessionID,
		SprintNumber:        report.SprintNumber,
		StartIteration:      report.StartIteration,
		EndIteration:        report.EndIteration,
		TasksAttempted:      report.TasksAttempted,
		TasksCompleted:      report.TasksCompleted,
		TasksFailed:         report.TasksFailed,
		Velocity:            report.Velocity,
		QualityTrend:        report.QualityTrend,
		TestPassRate:         report.TestPassRate,
		PatternsJSON:        string(patternsJSON),
		RecommendationsJSON: string(recsJSON),
		TotalTokens:         report.TotalTokens,
		DurationMs:          int(report.Duration.Milliseconds()),
	})
}

func severityFromFailCount(count int) string {
	if count >= 3 {
		return "high"
	}
	if count >= 2 {
		return "medium"
	}
	return "low"
}
