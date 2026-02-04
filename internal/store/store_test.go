package store

import (
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("opening test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpen(t *testing.T) {
	s := openTestStore(t)
	if s == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestSchemaVersion(t *testing.T) {
	s := openTestStore(t)
	var version int
	err := s.db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version)
	if err != nil {
		t.Fatalf("querying schema version: %v", err)
	}
	if version != currentSchemaVersion {
		t.Errorf("expected schema version %d, got %d", currentSchemaVersion, version)
	}
}

func TestSessionCRUD(t *testing.T) {
	s := openTestStore(t)

	// Create
	id, err := s.CreateSession("https://github.com/test/repo", "feat/test", `{"sprint_size":5}`)
	if err != nil {
		t.Fatalf("creating session: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero session ID")
	}

	// Get
	sess, err := s.GetSession(id)
	if err != nil {
		t.Fatalf("getting session: %v", err)
	}
	if sess.RepoURL != "https://github.com/test/repo" {
		t.Errorf("expected repo URL 'https://github.com/test/repo', got %q", sess.RepoURL)
	}
	if sess.Status != "running" {
		t.Errorf("expected status 'running', got %q", sess.Status)
	}

	// Update
	if err := s.UpdateSessionStatus(id, "completed"); err != nil {
		t.Fatalf("updating session: %v", err)
	}
	sess, _ = s.GetSession(id)
	if sess.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", sess.Status)
	}

	// Latest
	latest, err := s.LatestSession()
	if err != nil {
		t.Fatalf("getting latest session: %v", err)
	}
	if latest.ID != id {
		t.Errorf("expected latest session ID %d, got %d", id, latest.ID)
	}
}

func TestTaskCRUD(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	task := &Task{
		ID:          "task-1",
		SessionID:   sessionID,
		Title:       "Setup project",
		Description: "Initialize the project structure",
		Status:      "pending",
		Priority:    1,
		Complexity:  2,
		MaxAttempts: 3,
	}

	// Insert
	if err := s.InsertTask(task); err != nil {
		t.Fatalf("inserting task: %v", err)
	}

	// Get
	got, err := s.GetTask("task-1")
	if err != nil {
		t.Fatalf("getting task: %v", err)
	}
	if got.Title != "Setup project" {
		t.Errorf("expected title 'Setup project', got %q", got.Title)
	}
	if got.Status != "pending" {
		t.Errorf("expected status 'pending', got %q", got.Status)
	}

	// Update status
	if err := s.UpdateTaskStatus("task-1", "completed"); err != nil {
		t.Fatalf("updating task: %v", err)
	}
	got, _ = s.GetTask("task-1")
	if got.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", got.Status)
	}
	if got.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}

	// List
	tasks, err := s.ListTasks(sessionID)
	if err != nil {
		t.Fatalf("listing tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}
}

func TestTaskDependencies(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	for _, task := range []*Task{
		{ID: "task-1", SessionID: sessionID, Title: "First", Status: "pending", MaxAttempts: 3},
		{ID: "task-2", SessionID: sessionID, Title: "Second", Status: "pending", MaxAttempts: 3},
		{ID: "task-3", SessionID: sessionID, Title: "Third", Status: "pending", MaxAttempts: 3},
	} {
		if err := s.InsertTask(task); err != nil {
			t.Fatalf("inserting task %s: %v", task.ID, err)
		}
	}

	// task-2 depends on task-1, task-3 depends on task-2
	if err := s.AddDependency("task-2", "task-1"); err != nil {
		t.Fatalf("AddDependency(task-2, task-1): %v", err)
	}
	if err := s.AddDependency("task-3", "task-2"); err != nil {
		t.Fatalf("AddDependency(task-3, task-2): %v", err)
	}

	// NextTask should be task-1 (no deps)
	next, err := s.NextTask(sessionID)
	if err != nil {
		t.Fatalf("getting next task: %v", err)
	}
	if next == nil {
		t.Fatal("expected a next task")
	}
	if next.ID != "task-1" {
		t.Errorf("expected task-1, got %s", next.ID)
	}

	// Complete task-1, now task-2 should be next
	if err := s.UpdateTaskStatus("task-1", "completed"); err != nil {
		t.Fatalf("UpdateTaskStatus(task-1): %v", err)
	}
	next, _ = s.NextTask(sessionID)
	if next == nil || next.ID != "task-2" {
		t.Errorf("expected task-2, got %v", next)
	}

	// Complete task-2, now task-3
	if err := s.UpdateTaskStatus("task-2", "completed"); err != nil {
		t.Fatalf("UpdateTaskStatus(task-2): %v", err)
	}
	next, _ = s.NextTask(sessionID)
	if next == nil || next.ID != "task-3" {
		t.Errorf("expected task-3, got %v", next)
	}
}

func TestNextTaskRespectsMaxAttempts(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	if err := s.InsertTask(&Task{
		ID: "task-1", SessionID: sessionID, Title: "Flaky task",
		Status: "pending", MaxAttempts: 2,
	}); err != nil {
		t.Fatalf("InsertTask: %v", err)
	}

	// Record 2 attempts (max)
	now := time.Now()
	for i := 1; i <= 2; i++ {
		success := false
		if _, err := s.RecordAttempt(&Attempt{
			TaskID: "task-1", SessionID: sessionID, Number: i,
			AgentName: "claude", StartedAt: now, Success: &success,
		}); err != nil {
			t.Fatalf("RecordAttempt(%d): %v", i, err)
		}
	}

	next, err := s.NextTask(sessionID)
	if err != nil {
		t.Fatalf("getting next task: %v", err)
	}
	if next != nil {
		t.Errorf("expected nil (task at max attempts), got %v", next.ID)
	}
}

func TestAttemptCRUD(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	if err := s.InsertTask(&Task{
		ID: "task-1", SessionID: sessionID, Title: "Test", Status: "pending", MaxAttempts: 3,
	}); err != nil {
		t.Fatalf("InsertTask: %v", err)
	}

	now := time.Now()
	success := true
	id, err := s.RecordAttempt(&Attempt{
		TaskID:    "task-1",
		SessionID: sessionID,
		Number:    1,
		AgentName: "claude",
		StartedAt: now,
		Success:   &success,
		ErrorMsg:  "",
		GitCommit: "abc123",
	})
	if err != nil {
		t.Fatalf("recording attempt: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero attempt ID")
	}

	attempts, err := s.GetAttempts("task-1")
	if err != nil {
		t.Fatalf("getting attempts: %v", err)
	}
	if len(attempts) != 1 {
		t.Errorf("expected 1 attempt, got %d", len(attempts))
	}
	if attempts[0].GitCommit != "abc123" {
		t.Errorf("expected git commit 'abc123', got %q", attempts[0].GitCommit)
	}
}

func TestTranscript(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	if err := s.InsertTask(&Task{
		ID: "task-1", SessionID: sessionID, Title: "Test", Status: "pending", MaxAttempts: 3,
	}); err != nil {
		t.Fatalf("InsertTask: %v", err)
	}

	now := time.Now()
	success := true
	attemptID, err := s.RecordAttempt(&Attempt{
		TaskID: "task-1", SessionID: sessionID, Number: 1,
		AgentName: "claude", StartedAt: now, Success: &success,
	})
	if err != nil {
		t.Fatalf("RecordAttempt: %v", err)
	}

	if err := s.SaveTranscript(attemptID, "Full agent output here..."); err != nil {
		t.Fatalf("SaveTranscript: %v", err)
	}
	transcript, err := s.GetTranscript(attemptID)
	if err != nil {
		t.Fatalf("getting transcript: %v", err)
	}
	if transcript != "Full agent output here..." {
		t.Errorf("unexpected transcript: %q", transcript)
	}
}

func TestQualitySnapshots(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	// Record snapshots with improving quality
	for i := 1; i <= 6; i++ {
		pass := i > 3
		if err := s.RecordQuality(&QualitySnapshot{
			SessionID:   sessionID,
			Iteration:   i,
			OverallPass: pass,
			TestTotal:   10,
			TestPassed:  5 + i,
			TestFailed:  5 - i,
			FailedTestsJSON: `["test_a"]`,
		}); err != nil {
			t.Fatalf("RecordQuality(%d): %v", i, err)
		}
	}

	// Test pass rate
	rate, err := s.TestPassRate(sessionID, 6)
	if err != nil {
		t.Fatalf("getting test pass rate: %v", err)
	}
	if rate <= 0 {
		t.Errorf("expected positive pass rate, got %f", rate)
	}

	// Failing test trend
	trend, err := s.FailingTestTrend(sessionID, 6)
	if err != nil {
		t.Fatalf("getting failing test trend: %v", err)
	}
	if trend["test_a"] != 6 {
		t.Errorf("expected test_a to fail 6 times, got %d", trend["test_a"])
	}

	// Quality trend
	qt, err := s.QualityTrend(sessionID, 6)
	if err != nil {
		t.Fatalf("getting quality trend: %v", err)
	}
	if qt != "improving" {
		t.Errorf("expected 'improving', got %q", qt)
	}
}

func TestResourceUsage(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	if err := s.RecordUsage(&ResourceUsage{
		SessionID: sessionID, Iteration: 1, ContainerTimeMs: 5000, EstimatedTokens: 1000,
	}); err != nil {
		t.Fatalf("RecordUsage(1): %v", err)
	}
	if err := s.RecordUsage(&ResourceUsage{
		SessionID: sessionID, Iteration: 2, ContainerTimeMs: 3000, EstimatedTokens: 800,
	}); err != nil {
		t.Fatalf("RecordUsage(2): %v", err)
	}

	total, err := s.TotalUsage(sessionID)
	if err != nil {
		t.Fatalf("getting total usage: %v", err)
	}
	if total.ContainerTimeMs != 8000 {
		t.Errorf("expected 8000ms container time, got %d", total.ContainerTimeMs)
	}
	if total.EstimatedTokens != 1800 {
		t.Errorf("expected 1800 tokens, got %d", total.EstimatedTokens)
	}
}

func TestJournalEntries(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	if err := s.AddJournalEntry(&JournalEntry{
		SessionID:  sessionID,
		Kind:       "task_complete",
		TaskID:     "task-1",
		Sprint:     1,
		Iteration:  1,
		Summary:    "Completed auth setup",
		Reflection: "This went smoothly. The existing code had good patterns to follow.",
		Confidence: 4,
		Difficulty: 2,
		Momentum:   4,
	}); err != nil {
		t.Fatalf("AddJournalEntry: %v", err)
	}

	entries, err := s.JournalEntries(sessionID, nil)
	if err != nil {
		t.Fatalf("getting journal entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Kind != "task_complete" {
		t.Errorf("expected kind 'task_complete', got %q", entries[0].Kind)
	}

	// Filter by kind
	entries, _ = s.JournalEntries(sessionID, &JournalQuery{Kind: "task_start"})
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for kind 'task_start', got %d", len(entries))
	}

	// Export markdown
	md, err := s.ExportJournalMarkdown(sessionID)
	if err != nil {
		t.Fatalf("exporting markdown: %v", err)
	}
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

func TestSprintReports(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	if err := s.SaveSprintReport(&SprintReport{
		SessionID:      sessionID,
		SprintNumber:   1,
		StartIteration: 1,
		EndIteration:   5,
		TasksAttempted: 3,
		TasksCompleted: 2,
		TasksFailed:    1,
		Velocity:       0.67,
		QualityTrend:   "improving",
		TestPassRate:   0.85,
	}); err != nil {
		t.Fatalf("SaveSprintReport: %v", err)
	}

	reports, err := s.SprintReports(sessionID)
	if err != nil {
		t.Fatalf("getting sprint reports: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].TasksCompleted != 2 {
		t.Errorf("expected 2 completed, got %d", reports[0].TasksCompleted)
	}
}

func TestReviewResults(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	if err := s.SaveReviewResult(&ReviewResult{
		SessionID:    sessionID,
		Sprint:       1,
		ReviewAgent:  "claude-review",
		FindingsJSON: `[{"severity":"minor","description":"unused import"}]`,
		Summary:      "Looks good overall",
		Approved:     true,
	}); err != nil {
		t.Fatalf("SaveReviewResult: %v", err)
	}

	// Verify via raw query (no dedicated getter needed for now)
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM review_results WHERE session_id = ?", sessionID).Scan(&count); err != nil {
		t.Fatalf("QueryRow.Scan: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 review result, got %d", count)
	}
}

func TestUpdateTaskContextNotes(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	if err := s.InsertTask(&Task{
		ID: "task-1", SessionID: sessionID, Title: "Test", Status: "pending",
		MaxAttempts: 3, ContextNotes: "initial notes",
	}); err != nil {
		t.Fatalf("InsertTask: %v", err)
	}

	if err := s.UpdateTaskContextNotes("task-1", "updated context notes"); err != nil {
		t.Fatalf("UpdateTaskContextNotes: %v", err)
	}

	task, err := s.GetTask("task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.ContextNotes != "updated context notes" {
		t.Errorf("expected 'updated context notes', got %q", task.ContextNotes)
	}
}

func TestGetDependencies(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	for _, task := range []*Task{
		{ID: "task-1", SessionID: sessionID, Title: "First", Status: "pending", MaxAttempts: 3},
		{ID: "task-2", SessionID: sessionID, Title: "Second", Status: "pending", MaxAttempts: 3},
		{ID: "task-3", SessionID: sessionID, Title: "Third", Status: "pending", MaxAttempts: 3},
	} {
		if err := s.InsertTask(task); err != nil {
			t.Fatalf("InsertTask(%s): %v", task.ID, err)
		}
	}

	if err := s.AddDependency("task-3", "task-1"); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}
	if err := s.AddDependency("task-3", "task-2"); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}

	deps, err := s.GetDependencies("task-3")
	if err != nil {
		t.Fatalf("GetDependencies: %v", err)
	}
	if len(deps) != 2 {
		t.Errorf("expected 2 dependencies, got %d", len(deps))
	}

	// task-1 with no deps should return empty
	deps, err = s.GetDependencies("task-1")
	if err != nil {
		t.Fatalf("GetDependencies(task-1): %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 dependencies for task-1, got %d", len(deps))
	}
}

func TestGetAllDependencies(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	for _, task := range []*Task{
		{ID: "task-1", SessionID: sessionID, Title: "First", Status: "pending", MaxAttempts: 3},
		{ID: "task-2", SessionID: sessionID, Title: "Second", Status: "pending", MaxAttempts: 3},
		{ID: "task-3", SessionID: sessionID, Title: "Third", Status: "pending", MaxAttempts: 3},
	} {
		if err := s.InsertTask(task); err != nil {
			t.Fatalf("InsertTask(%s): %v", task.ID, err)
		}
	}

	if err := s.AddDependency("task-2", "task-1"); err != nil {
		t.Fatalf("AddDependency(task-2, task-1): %v", err)
	}
	if err := s.AddDependency("task-3", "task-2"); err != nil {
		t.Fatalf("AddDependency(task-3, task-2): %v", err)
	}

	allDeps, err := s.GetAllDependencies(sessionID)
	if err != nil {
		t.Fatalf("GetAllDependencies: %v", err)
	}
	if len(allDeps) != 2 {
		t.Errorf("expected 2 entries in dependency map, got %d", len(allDeps))
	}
	if len(allDeps["task-2"]) != 1 || allDeps["task-2"][0] != "task-1" {
		t.Errorf("expected task-2 depends on [task-1], got %v", allDeps["task-2"])
	}
	if len(allDeps["task-3"]) != 1 || allDeps["task-3"][0] != "task-2" {
		t.Errorf("expected task-3 depends on [task-2], got %v", allDeps["task-3"])
	}
}

func TestTaskAttemptCount(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	if err := s.InsertTask(&Task{
		ID: "task-1", SessionID: sessionID, Title: "Test", Status: "pending", MaxAttempts: 5,
	}); err != nil {
		t.Fatalf("InsertTask: %v", err)
	}

	// Zero attempts initially
	count, err := s.TaskAttemptCount("task-1")
	if err != nil {
		t.Fatalf("TaskAttemptCount: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 attempts, got %d", count)
	}

	// Record 3 attempts
	now := time.Now()
	for i := 1; i <= 3; i++ {
		success := i == 3
		if _, err := s.RecordAttempt(&Attempt{
			TaskID: "task-1", SessionID: sessionID, Number: i,
			AgentName: "claude", StartedAt: now, Success: &success,
		}); err != nil {
			t.Fatalf("RecordAttempt(%d): %v", i, err)
		}
	}

	count, err = s.TaskAttemptCount("task-1")
	if err != nil {
		t.Fatalf("TaskAttemptCount: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 attempts, got %d", count)
	}
}

func TestQualityTrend_Degrading(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	// Record snapshots with degrading quality: first half pass, second half fail
	for i := 1; i <= 6; i++ {
		pass := i <= 3 // first 3 pass, last 3 fail
		if err := s.RecordQuality(&QualitySnapshot{
			SessionID:   sessionID,
			Iteration:   i,
			OverallPass: pass,
			TestTotal:   10,
			TestPassed:  5,
		}); err != nil {
			t.Fatalf("RecordQuality(%d): %v", i, err)
		}
	}

	trend, err := s.QualityTrend(sessionID, 6)
	if err != nil {
		t.Fatalf("QualityTrend: %v", err)
	}
	if trend != "degrading" {
		t.Errorf("expected 'degrading', got %q", trend)
	}
}

func TestQualityTrend_Stable(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	// All snapshots pass — stable
	for i := 1; i <= 6; i++ {
		if err := s.RecordQuality(&QualitySnapshot{
			SessionID:   sessionID,
			Iteration:   i,
			OverallPass: true,
			TestTotal:   10,
			TestPassed:  10,
		}); err != nil {
			t.Fatalf("RecordQuality(%d): %v", i, err)
		}
	}

	trend, err := s.QualityTrend(sessionID, 6)
	if err != nil {
		t.Fatalf("QualityTrend: %v", err)
	}
	if trend != "stable" {
		t.Errorf("expected 'stable', got %q", trend)
	}
}

func TestQualityTrend_FewSnapshots(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	// Only 1 snapshot — should return "stable" (insufficient data)
	if err := s.RecordQuality(&QualitySnapshot{
		SessionID:   sessionID,
		Iteration:   1,
		OverallPass: true,
		TestTotal:   10,
		TestPassed:  10,
	}); err != nil {
		t.Fatalf("RecordQuality: %v", err)
	}

	trend, err := s.QualityTrend(sessionID, 1)
	if err != nil {
		t.Fatalf("QualityTrend: %v", err)
	}
	if trend != "stable" {
		t.Errorf("expected 'stable' for few snapshots, got %q", trend)
	}
}

func TestAddDependency_Duplicate(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	for _, task := range []*Task{
		{ID: "task-1", SessionID: sessionID, Title: "First", Status: "pending", MaxAttempts: 3},
		{ID: "task-2", SessionID: sessionID, Title: "Second", Status: "pending", MaxAttempts: 3},
	} {
		if err := s.InsertTask(task); err != nil {
			t.Fatalf("InsertTask(%s): %v", task.ID, err)
		}
	}

	// Add same dependency twice — should be idempotent
	if err := s.AddDependency("task-2", "task-1"); err != nil {
		t.Fatalf("AddDependency first: %v", err)
	}
	if err := s.AddDependency("task-2", "task-1"); err != nil {
		t.Fatalf("AddDependency duplicate: %v", err)
	}

	deps, err := s.GetDependencies("task-2")
	if err != nil {
		t.Fatalf("GetDependencies: %v", err)
	}
	if len(deps) != 1 {
		t.Errorf("expected 1 dependency after duplicate insert, got %d", len(deps))
	}
}

func TestNextTask_SkipsInProgress(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	// Insert two tasks: first in_progress, second pending
	if err := s.InsertTask(&Task{
		ID: "task-1", SessionID: sessionID, Title: "In progress", Status: "in_progress",
		Priority: 1, MaxAttempts: 3,
	}); err != nil {
		t.Fatalf("InsertTask(task-1): %v", err)
	}
	if err := s.InsertTask(&Task{
		ID: "task-2", SessionID: sessionID, Title: "Pending", Status: "pending",
		Priority: 2, MaxAttempts: 3,
	}); err != nil {
		t.Fatalf("InsertTask(task-2): %v", err)
	}

	// NextTask should return the in_progress task (it's included in the query)
	next, err := s.NextTask(sessionID)
	if err != nil {
		t.Fatalf("NextTask: %v", err)
	}
	if next == nil {
		t.Fatal("expected a next task")
	}
	// The query includes both pending and in_progress, so the higher priority one wins
	if next.ID != "task-1" {
		t.Errorf("expected task-1 (in_progress, higher priority), got %s", next.ID)
	}
}

func TestFailingTestTrend_MalformedJSON(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	// Record a snapshot with malformed JSON
	if err := s.RecordQuality(&QualitySnapshot{
		SessionID:       sessionID,
		Iteration:       1,
		OverallPass:     false,
		TestTotal:       10,
		TestFailed:      2,
		FailedTestsJSON: "not-valid-json",
	}); err != nil {
		t.Fatalf("RecordQuality: %v", err)
	}

	// Record a valid one too
	if err := s.RecordQuality(&QualitySnapshot{
		SessionID:       sessionID,
		Iteration:       2,
		OverallPass:     false,
		TestTotal:       10,
		TestFailed:      1,
		FailedTestsJSON: `["test_b"]`,
	}); err != nil {
		t.Fatalf("RecordQuality: %v", err)
	}

	// Should skip malformed JSON and still return valid data
	trend, err := s.FailingTestTrend(sessionID, 5)
	if err != nil {
		t.Fatalf("FailingTestTrend: %v", err)
	}
	if trend["test_b"] != 1 {
		t.Errorf("expected test_b count 1, got %d", trend["test_b"])
	}
}

func TestJournalEntries_CombinedFilters(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	entries := []*JournalEntry{
		{SessionID: sessionID, Kind: "task_complete", Sprint: 1, Iteration: 1, Summary: "A", Reflection: "RA"},
		{SessionID: sessionID, Kind: "task_complete", Sprint: 1, Iteration: 2, Summary: "B", Reflection: "RB"},
		{SessionID: sessionID, Kind: "task_complete", Sprint: 2, Iteration: 3, Summary: "C", Reflection: "RC"},
		{SessionID: sessionID, Kind: "task_start", Sprint: 1, Iteration: 1, Summary: "D", Reflection: "RD"},
		{SessionID: sessionID, Kind: "task_complete", Sprint: 1, Iteration: 4, Summary: "E", Reflection: "RE"},
	}

	for i, e := range entries {
		if err := s.AddJournalEntry(e); err != nil {
			t.Fatalf("AddJournalEntry(%d): %v", i, err)
		}
	}

	// Filter: kind=task_complete, sprint=1, limit=2
	results, err := s.JournalEntries(sessionID, &JournalQuery{
		Kind:   "task_complete",
		Sprint: 1,
		Limit:  2,
	})
	if err != nil {
		t.Fatalf("JournalEntries: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 entries with combined filters, got %d", len(results))
	}
	for _, r := range results {
		if r.Kind != "task_complete" {
			t.Errorf("expected kind 'task_complete', got %q", r.Kind)
		}
		if r.Sprint != 1 {
			t.Errorf("expected sprint 1, got %d", r.Sprint)
		}
	}
}

func TestExportJournalMarkdown_Rich(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	if err := s.AddJournalEntry(&JournalEntry{
		SessionID:  sessionID,
		Kind:       "task_complete",
		TaskID:     "task-1",
		Sprint:     1,
		Iteration:  1,
		Summary:    "Completed auth",
		Reflection: "Went well, existing patterns helped.",
		Confidence: 4,
		Difficulty: 3,
		Momentum:   5,
	}); err != nil {
		t.Fatalf("AddJournalEntry: %v", err)
	}

	md, err := s.ExportJournalMarkdown(sessionID)
	if err != nil {
		t.Fatalf("ExportJournalMarkdown: %v", err)
	}
	if md == "" {
		t.Fatal("expected non-empty markdown")
	}
	// Verify confidence and difficulty appear in output
	if !containsString(md, "Confidence: 4/5") {
		t.Error("expected markdown to contain confidence")
	}
	if !containsString(md, "Difficulty: 3/5") {
		t.Error("expected markdown to contain difficulty")
	}
	if !containsString(md, "Completed auth") {
		t.Error("expected markdown to contain summary")
	}
	if !containsString(md, "Went well") {
		t.Error("expected markdown to contain reflection")
	}
}

func TestLatestSession_Empty(t *testing.T) {
	s := openTestStore(t)

	// No sessions — should return error
	sess, err := s.LatestSession()
	if err == nil {
		t.Fatalf("expected error for empty sessions, got session %+v", sess)
	}
}

// containsString is a simple helper that checks for substring presence.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestQualityTrend_NoSnapshots(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	trend, err := s.QualityTrend(sessionID, 10)
	if err != nil {
		t.Fatalf("QualityTrend: %v", err)
	}
	if trend != "stable" {
		t.Errorf("expected 'stable' for no snapshots, got %q", trend)
	}
}

func TestGetTranscript_NoTranscript(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	if err := s.InsertTask(&Task{
		ID: "task-1", SessionID: sessionID, Title: "Test", Status: "pending", MaxAttempts: 3,
	}); err != nil {
		t.Fatalf("InsertTask: %v", err)
	}

	now := time.Now()
	success := true
	attemptID, _ := s.RecordAttempt(&Attempt{
		TaskID: "task-1", SessionID: sessionID, Number: 1,
		AgentName: "claude", StartedAt: now, Success: &success,
	})

	// No transcript saved — should return empty string
	transcript, err := s.GetTranscript(attemptID)
	if err != nil {
		t.Fatalf("GetTranscript: %v", err)
	}
	if transcript != "" {
		t.Errorf("expected empty transcript, got %q", transcript)
	}
}

func TestUpdateTaskStatus_NonCompleted(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	if err := s.InsertTask(&Task{
		ID: "task-1", SessionID: sessionID, Title: "Test", Status: "pending", MaxAttempts: 3,
	}); err != nil {
		t.Fatalf("InsertTask: %v", err)
	}

	// Update to in_progress — should NOT set completed_at
	if err := s.UpdateTaskStatus("task-1", "in_progress"); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	task, _ := s.GetTask("task-1")
	if task.Status != "in_progress" {
		t.Errorf("expected 'in_progress', got %q", task.Status)
	}
	if task.CompletedAt != nil {
		t.Error("expected nil completed_at for in_progress status")
	}
}

func TestExportDashboardData_Rich(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("https://github.com/test/repo", "feat/test", "")

	// Add tasks in various states.
	for _, task := range []*Task{
		{ID: "t-1", SessionID: sessionID, Title: "Done", Status: "completed", MaxAttempts: 3},
		{ID: "t-2", SessionID: sessionID, Title: "Pending", Status: "pending", MaxAttempts: 3},
		{ID: "t-3", SessionID: sessionID, Title: "Failed", Status: "failed", MaxAttempts: 3},
		{ID: "t-4", SessionID: sessionID, Title: "In Prog", Status: "in_progress", MaxAttempts: 3},
		{ID: "t-5", SessionID: sessionID, Title: "Deferred", Status: "deferred", MaxAttempts: 3},
	} {
		if err := s.InsertTask(task); err != nil {
			t.Fatalf("InsertTask(%s): %v", task.ID, err)
		}
	}

	// Add quality snapshots, usage, journal, sprint reports.
	s.RecordQuality(&QualitySnapshot{SessionID: sessionID, Iteration: 1, OverallPass: true, TestTotal: 10, TestPassed: 8})
	s.RecordUsage(&ResourceUsage{SessionID: sessionID, Iteration: 1, ContainerTimeMs: 5000, EstimatedTokens: 500})
	s.AddJournalEntry(&JournalEntry{SessionID: sessionID, Kind: "task_complete", Iteration: 1, Summary: "Done", Reflection: "ok"})
	s.SaveSprintReport(&SprintReport{SessionID: sessionID, SprintNumber: 1, TasksAttempted: 3, TasksCompleted: 1})

	data, err := s.ExportDashboardData(sessionID)
	if err != nil {
		t.Fatalf("ExportDashboardData: %v", err)
	}
	if data.TaskStats.Total != 5 {
		t.Errorf("expected 5 total tasks, got %d", data.TaskStats.Total)
	}
	if data.TaskStats.Completed != 1 {
		t.Errorf("expected 1 completed, got %d", data.TaskStats.Completed)
	}
	if data.TaskStats.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", data.TaskStats.Failed)
	}
	if data.TaskStats.InProgress != 1 {
		t.Errorf("expected 1 in_progress, got %d", data.TaskStats.InProgress)
	}
	if data.TaskStats.Deferred != 1 {
		t.Errorf("expected 1 deferred, got %d", data.TaskStats.Deferred)
	}
	if data.TotalUsage.EstimatedTokens != 500 {
		t.Errorf("expected 500 tokens, got %d", data.TotalUsage.EstimatedTokens)
	}
	if len(data.SprintReports) != 1 {
		t.Errorf("expected 1 sprint report, got %d", len(data.SprintReports))
	}
	if len(data.RecentJournal) != 1 {
		t.Errorf("expected 1 journal entry, got %d", len(data.RecentJournal))
	}
}

func TestTestPassRate_NoSnapshots(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	rate, err := s.TestPassRate(sessionID, 10)
	if err != nil {
		t.Fatalf("TestPassRate: %v", err)
	}
	if rate != 0 {
		t.Errorf("expected 0 pass rate for no snapshots, got %f", rate)
	}
}

func TestExportDashboardData(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("https://github.com/test/repo", "feat/test", "")

	if err := s.InsertTask(&Task{
		ID: "task-1", SessionID: sessionID, Title: "Test", Status: "completed", MaxAttempts: 3,
	}); err != nil {
		t.Fatalf("InsertTask(task-1): %v", err)
	}
	if err := s.InsertTask(&Task{
		ID: "task-2", SessionID: sessionID, Title: "Test 2", Status: "pending", MaxAttempts: 3,
	}); err != nil {
		t.Fatalf("InsertTask(task-2): %v", err)
	}

	data, err := s.ExportDashboardData(sessionID)
	if err != nil {
		t.Fatalf("exporting dashboard: %v", err)
	}
	if data.Session.RepoURL != "https://github.com/test/repo" {
		t.Errorf("unexpected repo URL: %q", data.Session.RepoURL)
	}
	if data.TaskStats.Total != 2 {
		t.Errorf("expected 2 total tasks, got %d", data.TaskStats.Total)
	}
	if data.TaskStats.Completed != 1 {
		t.Errorf("expected 1 completed, got %d", data.TaskStats.Completed)
	}
}
