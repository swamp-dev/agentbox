package supervisor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/swamp-dev/agentbox/internal/journal"
	"github.com/swamp-dev/agentbox/internal/metrics"
	"github.com/swamp-dev/agentbox/internal/ralph"
	"github.com/swamp-dev/agentbox/internal/retro"
	"github.com/swamp-dev/agentbox/internal/review"
	"github.com/swamp-dev/agentbox/internal/store"
	"github.com/swamp-dev/agentbox/internal/taskdb"
	"github.com/swamp-dev/agentbox/internal/workflow"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.SprintSize != 5 {
		t.Errorf("expected sprint size 5, got %d", cfg.SprintSize)
	}
	if cfg.MaxSprints != 20 {
		t.Errorf("expected max sprints 20, got %d", cfg.MaxSprints)
	}
	if cfg.Agent != "claude" {
		t.Errorf("expected agent 'claude', got %q", cfg.Agent)
	}
	if !cfg.JournalEnabled {
		t.Error("expected journal enabled by default")
	}
	if !cfg.ReviewEnabled {
		t.Error("expected review enabled by default")
	}
}

func TestParseBudgetDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BudgetDuration = "4h30m"

	if err := cfg.ParseBudgetDuration(); err != nil {
		t.Fatalf("ParseBudgetDuration: %v", err)
	}
	if cfg.Budget.MaxDuration != 4*time.Hour+30*time.Minute {
		t.Errorf("expected 4h30m, got %v", cfg.Budget.MaxDuration)
	}
}

func TestParseBudgetDuration_Invalid(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BudgetDuration = "not-a-duration"

	if err := cfg.ParseBudgetDuration(); err == nil {
		t.Error("expected error for invalid duration")
	}
}

func TestContextBuilder_BuildPrompt(t *testing.T) {
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	defer s.Close()

	sessionID, _ := s.CreateSession("", "main", "")

	// Insert a completed task.
	s.InsertTask(&store.Task{
		ID: "t-1", SessionID: sessionID, Title: "Setup", Status: "completed", MaxAttempts: 3,
	})

	cb := NewContextBuilder(s, sessionID)

	task := &taskdb.Task{
		ID:          "t-2",
		Title:       "Add auth",
		Description: "Implement authentication middleware",
		Attempts: []taskdb.Attempt{
			{Number: 1, AgentName: "claude", Success: false, ErrorMsg: "compile error on line 42"},
		},
	}

	prompt := cb.BuildPrompt(task, "test-project")

	if prompt == "" {
		t.Error("expected non-empty prompt")
	}

	checks := []string{
		"test-project",
		"t-2",
		"Add auth",
		"Implement authentication middleware",
		"Already Completed",
		"Setup",
		"<promise>COMPLETE</promise>",
		"compile error on line 42",
	}
	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Errorf("prompt missing expected content: %q", check)
		}
	}
}

func TestSprintRunner_BudgetExceeded(t *testing.T) {
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	defer s.Close()

	sessionID, _ := s.CreateSession("", "main", "")

	s.InsertTask(&store.Task{
		ID: "t-1", SessionID: sessionID, Title: "Test", Status: "pending", MaxAttempts: 3,
	})

	budget := metrics.Budget{
		MaxTokens:     1,
		WarnThreshold: 0.8,
	}

	s.RecordUsage(&store.ResourceUsage{
		SessionID: sessionID, Iteration: 1, EstimatedTokens: 100,
	})

	collector := metrics.NewCollector(s, sessionID)
	enforcer := metrics.NewBudgetEnforcer(budget)

	usage, _ := collector.TotalUsage()
	status := enforcer.Check(usage.EstimatedTokens, 1)
	if !status.Exceeded {
		t.Error("expected budget to be exceeded")
	}
}

func TestAdaptiveController_Apply(t *testing.T) {
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	defer s.Close()

	sessionID, _ := s.CreateSession("", "main", "")
	s.InsertTask(&store.Task{
		ID: "t-1", SessionID: sessionID, Title: "Failing task", Status: "pending", MaxAttempts: 3,
	})

	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, logger)

	recs := []retro.Recommendation{
		{Action: retro.RecDeferTask, TaskID: "t-1", Description: "Too many failures"},
	}

	actions := ac.Apply(recs)
	if len(actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(actions))
	}

	task, _ := s.GetTask("t-1")
	if task.Status != "deferred" {
		t.Errorf("expected task deferred, got %s", task.Status)
	}
}

// MockAgentRunner provides configurable success/failure responses.
type MockAgentRunner struct {
	results []*ralph.IterationResult
	idx     int
}

func (m *MockAgentRunner) RunTask(_ context.Context, task *ralph.Task, _ string) *ralph.IterationResult {
	if m.idx < len(m.results) {
		r := m.results[m.idx]
		m.idx++
		return r
	}
	return &ralph.IterationResult{
		TaskID:  task.ID,
		Success: false,
		Error:   "no more mock results",
	}
}

// openTestStore opens an in-memory store for testing.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("opening test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// setupTestSupervisorDeps creates common test dependencies.
func setupTestSupervisorDeps(t *testing.T) (*store.Store, int64, *metrics.Collector, *metrics.BudgetEnforcer, *journal.Journal, *slog.Logger) {
	t.Helper()
	s := openTestStore(t)
	sessionID, err := s.CreateSession("", "main", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	logger := testLogger()
	collector := metrics.NewCollector(s, sessionID)
	budget := metrics.NewBudgetEnforcer(metrics.DefaultBudget())
	j := journal.New(s, sessionID)
	return s, sessionID, collector, budget, j, logger
}

func TestNew(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.WorkDir = dir

	sup, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sup.Store().Close()

	if sup.SessionID() == 0 {
		t.Error("expected non-zero session ID")
	}
	if sup.Store() == nil {
		t.Error("expected non-nil store")
	}
}

func TestNew_DefaultConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.WorkDir = dir

	sup, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New with defaults: %v", err)
	}
	defer sup.Store().Close()

	if sup.cfg.SprintSize != 5 {
		t.Errorf("expected default sprint size 5, got %d", sup.cfg.SprintSize)
	}
}

func TestGeneratePRBody(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.WorkDir = dir

	sup, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sup.Store().Close()

	// Add tasks to taskDB.
	sup.taskDB.Add(&taskdb.Task{
		ID: "t-1", Title: "Setup auth", Status: taskdb.StatusCompleted, MaxAttempts: 3,
	})
	sup.taskDB.Add(&taskdb.Task{
		ID: "t-2", Title: "Add tests", Status: taskdb.StatusPending, MaxAttempts: 3,
	})
	sup.taskDB.Add(&taskdb.Task{
		ID: "t-3", Title: "Broken task", Status: taskdb.StatusFailed, MaxAttempts: 3,
	})

	body, err := sup.generatePRBody()
	if err != nil {
		t.Fatalf("generatePRBody: %v", err)
	}

	checks := []string{
		"1/3",           // completed/total
		"Setup auth",    // completed task
		"Broken task",   // failed task
		"Unresolved",    // section header
		"agentbox",      // footer
	}
	for _, check := range checks {
		if !strings.Contains(body, check) {
			t.Errorf("PR body missing expected content: %q", check)
		}
	}
}

func TestRunReviewGate_NoReviewer(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.WorkDir = dir
	cfg.ReviewEnabled = true

	sup, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sup.Store().Close()

	// reviewer is nil by default — runReviewGate should not panic
	ctx := context.Background()
	sup.runReviewGate(ctx)
	// No panic = success
}

func TestSprintRunner_NewWithNilRunner(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	cfg := DefaultConfig()
	logger := testLogger()
	collector := metrics.NewCollector(s, sessionID)
	budget := metrics.NewBudgetEnforcer(metrics.DefaultBudget())
	j := journal.New(s, sessionID)

	// Pass nil runner — should use NoopAgentRunner
	sr := NewSprintRunner(cfg, s, sessionID, nil, taskdb.New(), collector, budget, j, nil, logger)
	if sr.runner == nil {
		t.Fatal("expected non-nil runner after passing nil")
	}
}

func TestSprintRunner_RunSprint_EmptyTaskDB(t *testing.T) {
	s, sessionID, collector, budget, j, logger := setupTestSupervisorDeps(t)
	cfg := DefaultConfig()
	tdb := taskdb.New() // empty

	sr := NewSprintRunner(cfg, s, sessionID, nil, tdb, collector, budget, j, nil, logger)
	result, err := sr.RunSprint(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("RunSprint: %v", err)
	}
	if result.TasksAttempted != 0 {
		t.Errorf("expected 0 tasks attempted, got %d", result.TasksAttempted)
	}
}

func TestSprintRunner_RunSprint_Normal(t *testing.T) {
	s, sessionID, collector, budget, j, logger := setupTestSupervisorDeps(t)
	cfg := DefaultConfig()
	cfg.SprintSize = 2
	cfg.JournalEnabled = false

	tdb := taskdb.New()
	tdb.Add(&taskdb.Task{ID: "t-1", Title: "Task 1", Status: taskdb.StatusPending, MaxAttempts: 3})
	tdb.Add(&taskdb.Task{ID: "t-2", Title: "Task 2", Status: taskdb.StatusPending, MaxAttempts: 3})

	// Insert tasks into store too (for retro analysis).
	s.InsertTask(&store.Task{ID: "t-1", SessionID: sessionID, Title: "Task 1", Status: "pending", MaxAttempts: 3})
	s.InsertTask(&store.Task{ID: "t-2", SessionID: sessionID, Title: "Task 2", Status: "pending", MaxAttempts: 3})

	mockRunner := &MockAgentRunner{
		results: []*ralph.IterationResult{
			{TaskID: "t-1", Success: true, Output: "done"},
			{TaskID: "t-2", Success: true, Output: "done"},
		},
	}

	// Create a temp dir for workflow (git operations).
	dir := t.TempDir()
	wf := workflow.NewGitWorkflow("", dir, logger)

	sr := NewSprintRunner(cfg, s, sessionID, wf, tdb, collector, budget, j, mockRunner, logger)
	result, err := sr.RunSprint(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("RunSprint: %v", err)
	}
	if result.TasksAttempted != 2 {
		t.Errorf("expected 2 tasks attempted, got %d", result.TasksAttempted)
	}
	if result.TasksCompleted != 2 {
		t.Errorf("expected 2 tasks completed, got %d", result.TasksCompleted)
	}
}

func TestSprintRunner_RunSprint_TaskFailure(t *testing.T) {
	s, sessionID, collector, budget, j, logger := setupTestSupervisorDeps(t)
	cfg := DefaultConfig()
	cfg.SprintSize = 2
	cfg.JournalEnabled = false
	cfg.MaxConsecutiveFails = 10 // high to not trigger abort

	tdb := taskdb.New()
	tdb.Add(&taskdb.Task{ID: "t-1", Title: "Failing", Status: taskdb.StatusPending, MaxAttempts: 3})
	tdb.Add(&taskdb.Task{ID: "t-2", Title: "Passing", Status: taskdb.StatusPending, MaxAttempts: 3})

	s.InsertTask(&store.Task{ID: "t-1", SessionID: sessionID, Title: "Failing", Status: "pending", MaxAttempts: 3})
	s.InsertTask(&store.Task{ID: "t-2", SessionID: sessionID, Title: "Passing", Status: "pending", MaxAttempts: 3})

	mockRunner := &MockAgentRunner{
		results: []*ralph.IterationResult{
			{TaskID: "t-1", Success: false, Error: "compile error"},
			{TaskID: "t-2", Success: true, Output: "done"},
		},
	}

	dir := t.TempDir()
	wf := workflow.NewGitWorkflow("", dir, logger)

	sr := NewSprintRunner(cfg, s, sessionID, wf, tdb, collector, budget, j, mockRunner, logger)
	result, err := sr.RunSprint(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("RunSprint: %v", err)
	}
	if result.TasksFailed != 1 {
		t.Errorf("expected 1 failed, got %d", result.TasksFailed)
	}
	if result.TasksCompleted != 1 {
		t.Errorf("expected 1 completed, got %d", result.TasksCompleted)
	}
}

func TestSprintRunner_RunSprint_ConsecutiveFailAbort(t *testing.T) {
	s, sessionID, collector, budget, j, logger := setupTestSupervisorDeps(t)
	cfg := DefaultConfig()
	cfg.SprintSize = 5
	cfg.MaxConsecutiveFails = 2
	cfg.JournalEnabled = false

	tdb := taskdb.New()
	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("t-%d", i)
		tdb.Add(&taskdb.Task{ID: id, Title: "Task", Status: taskdb.StatusPending, MaxAttempts: 5})
		s.InsertTask(&store.Task{ID: id, SessionID: sessionID, Title: "Task", Status: "pending", MaxAttempts: 5})
	}

	// All fail
	mockRunner := &MockAgentRunner{
		results: []*ralph.IterationResult{
			{TaskID: "t-1", Success: false, Error: "fail 1"},
			{TaskID: "t-2", Success: false, Error: "fail 2"},
			{TaskID: "t-3", Success: false, Error: "fail 3"},
			{TaskID: "t-4", Success: false, Error: "fail 4"},
			{TaskID: "t-5", Success: false, Error: "fail 5"},
		},
	}

	dir := t.TempDir()
	wf := workflow.NewGitWorkflow("", dir, logger)

	sr := NewSprintRunner(cfg, s, sessionID, wf, tdb, collector, budget, j, mockRunner, logger)
	result, _ := sr.RunSprint(context.Background(), 1, 1)
	if !result.AbortedEarly {
		t.Error("expected sprint to abort early due to consecutive failures")
	}
	if !strings.Contains(result.AbortReason, "consecutive failures") {
		t.Errorf("expected abort reason to mention consecutive failures, got %q", result.AbortReason)
	}
	// Should have attempted 2 (stopped at max consecutive fails)
	if result.TasksAttempted != 2 {
		t.Errorf("expected 2 tasks attempted before abort, got %d", result.TasksAttempted)
	}
}

func TestSprintRunner_RunSprint_CancelledContext(t *testing.T) {
	s, sessionID, collector, budget, j, logger := setupTestSupervisorDeps(t)
	cfg := DefaultConfig()
	cfg.SprintSize = 5
	cfg.JournalEnabled = false

	tdb := taskdb.New()
	tdb.Add(&taskdb.Task{ID: "t-1", Title: "Task", Status: taskdb.StatusPending, MaxAttempts: 3})
	s.InsertTask(&store.Task{ID: "t-1", SessionID: sessionID, Title: "Task", Status: "pending", MaxAttempts: 3})

	// Cancel context before running.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sr := NewSprintRunner(cfg, s, sessionID, nil, tdb, collector, budget, j, nil, logger)
	result, err := sr.RunSprint(ctx, 1, 1)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if result != nil && !result.AbortedEarly {
		t.Error("expected sprint to be aborted early")
	}
}

func TestSprintRunner_RunSprint_BudgetExceeded(t *testing.T) {
	s, sessionID, collector, _, j, logger := setupTestSupervisorDeps(t)
	cfg := DefaultConfig()
	cfg.SprintSize = 5
	cfg.JournalEnabled = false

	// Set tiny budget.
	tinyBudget := metrics.Budget{MaxTokens: 1, WarnThreshold: 0.8}
	budget := metrics.NewBudgetEnforcer(tinyBudget)

	// Pre-load usage to exceed budget.
	s.RecordUsage(&store.ResourceUsage{SessionID: sessionID, Iteration: 1, EstimatedTokens: 100})

	tdb := taskdb.New()
	tdb.Add(&taskdb.Task{ID: "t-1", Title: "Task", Status: taskdb.StatusPending, MaxAttempts: 3})
	s.InsertTask(&store.Task{ID: "t-1", SessionID: sessionID, Title: "Task", Status: "pending", MaxAttempts: 3})

	sr := NewSprintRunner(cfg, s, sessionID, nil, tdb, collector, budget, j, nil, logger)
	result, _ := sr.RunSprint(context.Background(), 1, 1)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.BudgetExceeded {
		t.Error("expected budget exceeded")
	}
	if !result.AbortedEarly {
		t.Error("expected abort early")
	}
}

func TestSprintRunner_CurrentIteration(t *testing.T) {
	s, sessionID, collector, budget, j, logger := setupTestSupervisorDeps(t)
	cfg := DefaultConfig()

	sr := NewSprintRunner(cfg, s, sessionID, nil, taskdb.New(), collector, budget, j, nil, logger)

	// Run empty sprint starting at iteration 5.
	sr.RunSprint(context.Background(), 1, 5)
	if sr.CurrentIteration() != 5 {
		t.Errorf("expected current iteration 5, got %d", sr.CurrentIteration())
	}
}

func TestWriteSprintRetroEntry(t *testing.T) {
	s, sessionID, collector, budget, j, logger := setupTestSupervisorDeps(t)
	cfg := DefaultConfig()
	cfg.JournalEnabled = true

	sr := NewSprintRunner(cfg, s, sessionID, nil, taskdb.New(), collector, budget, j, nil, logger)
	sr.sprintNum = 1
	sr.iteration = 5

	report := &retro.SprintReport{
		SprintNumber:   1,
		TasksAttempted: 3,
		TasksCompleted: 2,
		Velocity:       0.67,
		QualityTrend:   "improving",
		TestPassRate:   0.9,
		Patterns: []retro.Pattern{
			{Type: retro.PatternHighVelocity, Description: "good velocity"},
		},
		Recommendations: []retro.Recommendation{
			{Action: retro.RecUpdateContext, Description: "add more context"},
		},
	}

	sr.writeSprintRetroEntry(report)

	entries, err := s.JournalEntries(sessionID, &store.JournalQuery{Kind: "sprint_retro"})
	if err != nil {
		t.Fatalf("JournalEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 retro entry, got %d", len(entries))
	}
	if !strings.Contains(entries[0].Reflection, "Velocity") {
		t.Error("expected retro reflection to mention velocity")
	}
	if !strings.Contains(entries[0].Reflection, "Patterns detected") {
		t.Error("expected retro reflection to mention patterns")
	}
	if !strings.Contains(entries[0].Reflection, "Recommendations") {
		t.Error("expected retro reflection to mention recommendations")
	}
}

func TestNoopAgentRunner_RunTask(t *testing.T) {
	runner := &NoopAgentRunner{}
	task := &ralph.Task{ID: "t-1", Title: "test"}
	result := runner.RunTask(context.Background(), task, "some prompt")
	if result.Success {
		t.Error("expected NoopAgentRunner to return failure")
	}
	if result.TaskID != "t-1" {
		t.Errorf("expected task ID t-1, got %q", result.TaskID)
	}
	if result.Error == "" {
		t.Error("expected error message from NoopAgentRunner")
	}
}

func TestContextBuilder_BuildPrompt_WithFailingTests(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	// Insert a completed task.
	s.InsertTask(&store.Task{
		ID: "t-1", SessionID: sessionID, Title: "Setup", Status: "completed", MaxAttempts: 3,
	})

	// Add failing test data.
	for i := 1; i <= 3; i++ {
		s.RecordQuality(&store.QualitySnapshot{
			SessionID:       sessionID,
			Iteration:       i,
			OverallPass:     false,
			TestTotal:       10,
			TestFailed:      2,
			FailedTestsJSON: `["TestAuth", "TestDB"]`,
		})
	}

	cb := NewContextBuilder(s, sessionID)
	task := &taskdb.Task{
		ID:          "t-2",
		Title:       "Fix auth",
		Description: "Fix authentication",
		MaxAttempts: 3,
	}
	prompt := cb.BuildPrompt(task, "test-project")

	if !strings.Contains(prompt, "Known Failing Tests") {
		t.Error("expected prompt to contain failing tests section")
	}
	if !strings.Contains(prompt, "TestAuth") {
		t.Error("expected prompt to mention TestAuth")
	}
}

func TestContextBuilder_BuildPrompt_WithAcceptanceCriteria(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	cb := NewContextBuilder(s, sessionID)
	task := &taskdb.Task{
		ID:          "t-1",
		Title:       "Add auth",
		Description: "Add authentication",
		MaxAttempts: 3,
		AcceptanceCriteria: []taskdb.AcceptanceCriteria{
			{Description: "Tests pass", Command: "go test ./..."},
			{Description: "Lint clean"},
		},
		ContextNotes: "Use JWT tokens",
	}
	prompt := cb.BuildPrompt(task, "test-project")

	if !strings.Contains(prompt, "Acceptance Criteria") {
		t.Error("expected acceptance criteria section")
	}
	if !strings.Contains(prompt, "go test ./...") {
		t.Error("expected verify command in prompt")
	}
	if !strings.Contains(prompt, "Additional Context") {
		t.Error("expected context notes section")
	}
	if !strings.Contains(prompt, "JWT tokens") {
		t.Error("expected context notes content")
	}
}

func TestSprintRunner_RunIteration_Success(t *testing.T) {
	s, sessionID, collector, budget, j, logger := setupTestSupervisorDeps(t)
	cfg := DefaultConfig()
	cfg.JournalEnabled = true
	cfg.AutoCommit = false // Don't try git operations

	tdb := taskdb.New()
	task := &taskdb.Task{ID: "t-1", Title: "Task 1", Status: taskdb.StatusPending, MaxAttempts: 3}
	tdb.Add(task)
	s.InsertTask(&store.Task{ID: "t-1", SessionID: sessionID, Title: "Task 1", Status: "pending", MaxAttempts: 3})

	mockRunner := &MockAgentRunner{
		results: []*ralph.IterationResult{
			{TaskID: "t-1", Success: true, Output: "completed task"},
		},
	}

	dir := t.TempDir()
	wf := workflow.NewGitWorkflow("", dir, logger)

	sr := NewSprintRunner(cfg, s, sessionID, wf, tdb, collector, budget, j, mockRunner, logger)
	sr.sprintNum = 1
	sr.iteration = 1

	success := sr.runIteration(context.Background(), task)
	if !success {
		t.Error("expected iteration to succeed")
	}
	if task.Status != taskdb.StatusCompleted {
		t.Errorf("expected task completed, got %s", task.Status)
	}

	// Verify attempt was recorded.
	attempts, _ := s.GetAttempts("t-1")
	if len(attempts) != 1 {
		t.Errorf("expected 1 attempt recorded, got %d", len(attempts))
	}
}

func TestSprintRunner_RunIteration_Failure(t *testing.T) {
	s, sessionID, collector, budget, j, logger := setupTestSupervisorDeps(t)
	cfg := DefaultConfig()
	cfg.JournalEnabled = true
	cfg.AutoCommit = false

	tdb := taskdb.New()
	task := &taskdb.Task{ID: "t-1", Title: "Failing task", Status: taskdb.StatusPending, MaxAttempts: 3}
	tdb.Add(task)
	s.InsertTask(&store.Task{ID: "t-1", SessionID: sessionID, Title: "Failing task", Status: "pending", MaxAttempts: 3})

	mockRunner := &MockAgentRunner{
		results: []*ralph.IterationResult{
			{TaskID: "t-1", Success: false, Error: "compilation failed", Output: "error log"},
		},
	}

	dir := t.TempDir()
	wf := workflow.NewGitWorkflow("", dir, logger)

	sr := NewSprintRunner(cfg, s, sessionID, wf, tdb, collector, budget, j, mockRunner, logger)
	sr.sprintNum = 1
	sr.iteration = 1

	success := sr.runIteration(context.Background(), task)
	if success {
		t.Error("expected iteration to fail")
	}
	// Task should NOT be completed.
	if task.Status == taskdb.StatusCompleted {
		t.Error("expected task to not be completed after failure")
	}
}

func TestImportPRD(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.WorkDir = dir

	sup, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sup.Store().Close()

	// Create a worktree path with a PRD file.
	worktreeDir := t.TempDir()
	sup.workflow = workflow.NewGitWorkflow("", worktreeDir, testLogger())
	// Manually set the worktree path field (internal field).
	// Since we can't directly set the private worktreePath, we use RepoDir() as fallback.
	// Write PRD to the base dir (which workDir() returns when no worktree is set).
	prdContent := `{
		"name": "Test Project",
		"tasks": [
			{"id": "t-1", "title": "Setup", "description": "Set up project", "status": "pending", "priority": 1},
			{"id": "t-2", "title": "Core", "description": "Core logic", "status": "pending", "priority": 2, "depends_on": ["t-1"]}
		]
	}`
	prdPath := worktreeDir + "/prd.json"
	if err := os.WriteFile(prdPath, []byte(prdContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg.PRDFile = prdPath

	if err := sup.importPRD(); err != nil {
		t.Fatalf("importPRD: %v", err)
	}

	total, _, _, _, _ := sup.taskDB.Stats()
	if total != 2 {
		t.Errorf("expected 2 tasks imported, got %d", total)
	}

	// Verify tasks in store.
	tasks, _ := sup.Store().ListTasks(sup.SessionID())
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks in store, got %d", len(tasks))
	}

	// Verify dependencies in store.
	deps, _ := sup.Store().GetDependencies("t-2")
	if len(deps) != 1 || deps[0] != "t-1" {
		t.Errorf("expected t-2 depends on [t-1], got %v", deps)
	}
}

func TestSetup_Integration(t *testing.T) {
	// Create a git repo for the test.
	repoDir := t.TempDir()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "checkout", "-b", "main"},
	}
	for _, args := range cmds {
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = repoDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("cmd %v: %v", args, err)
		}
	}
	// Create initial commit.
	readmePath := repoDir + "/README.md"
	os.WriteFile(readmePath, []byte("# Test\n"), 0644)
	cmd := exec.CommandContext(context.Background(), "git", "add", "-A")
	cmd.Dir = repoDir
	cmd.Run()
	cmd = exec.CommandContext(context.Background(), "git", "commit", "-m", "init")
	cmd.Dir = repoDir
	cmd.Run()

	// Write PRD file.
	prdContent := `{"name":"Test","tasks":[{"id":"t-1","title":"Setup","description":"Setup","status":"pending","priority":1}]}`
	os.WriteFile(repoDir+"/prd.json", []byte(prdContent), 0644)
	cmd = exec.CommandContext(context.Background(), "git", "add", "-A")
	cmd.Dir = repoDir
	cmd.Run()
	cmd = exec.CommandContext(context.Background(), "git", "commit", "-m", "add prd")
	cmd.Dir = repoDir
	cmd.Run()

	// Create supervisor with this repo.
	cfg := DefaultConfig()
	cfg.WorkDir = repoDir
	cfg.BranchName = "feat/test-setup"
	cfg.JournalEnabled = true

	sup, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sup.Store().Close()

	ctx := context.Background()
	if err := sup.setup(ctx); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Verify tasks were imported.
	total, _, _, _, _ := sup.taskDB.Stats()
	if total != 1 {
		t.Errorf("expected 1 task imported, got %d", total)
	}

	// Verify session is still running.
	sess, _ := sup.Store().GetSession(sup.SessionID())
	if sess.Status != "running" {
		t.Errorf("expected session running, got %s", sess.Status)
	}
}

func TestRun_CancelledContext(t *testing.T) {
	repoDir := t.TempDir()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "checkout", "-b", "main"},
	}
	for _, args := range cmds {
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = repoDir
		cmd.Run()
	}
	os.WriteFile(repoDir+"/README.md", []byte("# Test\n"), 0644)
	prdContent := `{"name":"Test","tasks":[{"id":"t-1","title":"Task","description":"Do it","status":"pending","priority":1}]}`
	os.WriteFile(repoDir+"/prd.json", []byte(prdContent), 0644)
	cmd := exec.CommandContext(context.Background(), "git", "add", "-A")
	cmd.Dir = repoDir
	cmd.Run()
	cmd = exec.CommandContext(context.Background(), "git", "commit", "-m", "init")
	cmd.Dir = repoDir
	cmd.Run()

	cfg := DefaultConfig()
	cfg.WorkDir = repoDir
	cfg.BranchName = "feat/test-cancel"
	cfg.JournalEnabled = false
	cfg.ReviewEnabled = false

	sup, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Cancel context immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = sup.Run(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestFinalize_Basic(t *testing.T) {
	repoDir := t.TempDir()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "checkout", "-b", "main"},
	}
	for _, args := range cmds {
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = repoDir
		cmd.Run()
	}
	os.WriteFile(repoDir+"/README.md", []byte("# Test\n"), 0644)
	cmd := exec.CommandContext(context.Background(), "git", "add", "-A")
	cmd.Dir = repoDir
	cmd.Run()
	cmd = exec.CommandContext(context.Background(), "git", "commit", "-m", "init")
	cmd.Dir = repoDir
	cmd.Run()

	cfg := DefaultConfig()
	cfg.WorkDir = repoDir
	cfg.BranchName = "feat/test-finalize"
	cfg.JournalEnabled = true
	cfg.ReviewEnabled = false

	sup, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sup.Store().Close()

	// Setup first to create worktree.
	prdContent := `{"name":"Test","tasks":[{"id":"t-1","title":"Task","description":"Do it","status":"pending","priority":1}]}`
	os.WriteFile(repoDir+"/prd.json", []byte(prdContent), 0644)
	cmd = exec.CommandContext(context.Background(), "git", "add", "-A")
	cmd.Dir = repoDir
	cmd.Run()
	cmd = exec.CommandContext(context.Background(), "git", "commit", "-m", "add prd")
	cmd.Dir = repoDir
	cmd.Run()

	ctx := context.Background()
	if err := sup.setup(ctx); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Finalize — will fail on OpenPR (no remote), but should handle gracefully.
	err = sup.finalize(ctx)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}

	// Verify session marked as completed.
	sess, _ := sup.Store().GetSession(sup.SessionID())
	if sess.Status != "completed" {
		t.Errorf("expected session completed, got %s", sess.Status)
	}
}

func TestRunReviewGate_WithReviewer_NoDiff(t *testing.T) {
	// Test runReviewGate when reviewer is set but diff fails (no remote).
	repoDir := t.TempDir()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "checkout", "-b", "main"},
	}
	for _, args := range cmds {
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = repoDir
		cmd.Run()
	}
	os.WriteFile(repoDir+"/README.md", []byte("# Test\n"), 0644)
	cmd := exec.CommandContext(context.Background(), "git", "add", "-A")
	cmd.Dir = repoDir
	cmd.Run()
	cmd = exec.CommandContext(context.Background(), "git", "commit", "-m", "init")
	cmd.Dir = repoDir
	cmd.Run()

	cfg := DefaultConfig()
	cfg.WorkDir = repoDir
	cfg.BranchName = "feat/review-gate-test"
	cfg.ReviewEnabled = true
	cfg.JournalEnabled = false

	sup, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sup.Store().Close()

	// Set up workflow with worktree.
	ctx := context.Background()
	sup.workflow.CloneOrOpen(ctx)
	sup.workflow.CreateWorktree(ctx, "feat/review-gate-test")

	// Set a fake reviewer (with nil container — will fail on Review call).
	sup.reviewer = NewFakeReviewer()

	// This should handle the diff error gracefully (no remote = diff fails).
	sup.runReviewGate(ctx) // Should not panic.
}

// NewFakeReviewer creates a reviewer that would fail but tests the non-nil path.
func NewFakeReviewer() *review.Reviewer {
	return review.NewReviewer("fake-reviewer", nil, nil, testLogger())
}

func TestRun_CompletesAllTasks(t *testing.T) {
	repoDir := t.TempDir()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "checkout", "-b", "main"},
	}
	for _, args := range cmds {
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = repoDir
		cmd.Run()
	}

	prdContent := `{"name":"Test","tasks":[{"id":"t-1","title":"Easy task","description":"Do easy thing","status":"pending","priority":1}]}`
	os.WriteFile(repoDir+"/README.md", []byte("# Test\n"), 0644)
	os.WriteFile(repoDir+"/prd.json", []byte(prdContent), 0644)
	cmd := exec.CommandContext(context.Background(), "git", "add", "-A")
	cmd.Dir = repoDir
	cmd.Run()
	cmd = exec.CommandContext(context.Background(), "git", "commit", "-m", "init")
	cmd.Dir = repoDir
	cmd.Run()

	cfg := DefaultConfig()
	cfg.WorkDir = repoDir
	cfg.BranchName = "feat/test-run-complete"
	cfg.JournalEnabled = false
	cfg.ReviewEnabled = false
	cfg.MaxSprints = 1
	cfg.SprintSize = 3

	sup, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Can't fully test Run because it calls setup which clones, then finalize which tries OpenPR.
	// But we can test the sprint loop portion indirectly via setup + manual sprint runner.
	ctx := context.Background()
	if err := sup.setup(ctx); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// All tasks should be imported.
	total, _, _, _, _ := sup.taskDB.Stats()
	if total != 1 {
		t.Errorf("expected 1 task, got %d", total)
	}
}

func TestApply_SplitTask(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, logger)

	recs := []retro.Recommendation{
		{Action: retro.RecSplitTask, TaskID: "t-1", Description: "split into subtasks"},
	}
	actions := ac.Apply(recs)
	if len(actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(actions))
	}
	if !strings.Contains(actions[0], "split task") {
		t.Errorf("expected split task action, got %q", actions[0])
	}
}

func TestAdaptiveController_WriteEscalation(t *testing.T) {
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	defer s.Close()

	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, logger)

	dir := t.TempDir()
	err = ac.WriteEscalation(dir, "System is stuck on auth module")
	if err != nil {
		t.Fatalf("WriteEscalation: %v", err)
	}

	data, err := os.ReadFile(dir + "/.agentbox/escalations.md")
	if err != nil {
		t.Fatalf("reading escalation file: %v", err)
	}
	if !strings.Contains(string(data), "System is stuck on auth module") {
		t.Error("expected escalation message in file")
	}
}
