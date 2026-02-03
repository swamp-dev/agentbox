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
	s.AddDependency("task-2", "task-1")
	s.AddDependency("task-3", "task-2")

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
	s.UpdateTaskStatus("task-1", "completed")
	next, _ = s.NextTask(sessionID)
	if next == nil || next.ID != "task-2" {
		t.Errorf("expected task-2, got %v", next)
	}

	// Complete task-2, now task-3
	s.UpdateTaskStatus("task-2", "completed")
	next, _ = s.NextTask(sessionID)
	if next == nil || next.ID != "task-3" {
		t.Errorf("expected task-3, got %v", next)
	}
}

func TestNextTaskRespectsMaxAttempts(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	s.InsertTask(&Task{
		ID: "task-1", SessionID: sessionID, Title: "Flaky task",
		Status: "pending", MaxAttempts: 2,
	})

	// Record 2 attempts (max)
	now := time.Now()
	for i := 1; i <= 2; i++ {
		success := false
		s.RecordAttempt(&Attempt{
			TaskID: "task-1", SessionID: sessionID, Number: i,
			AgentName: "claude", StartedAt: now, Success: &success,
		})
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
	s.InsertTask(&Task{
		ID: "task-1", SessionID: sessionID, Title: "Test", Status: "pending", MaxAttempts: 3,
	})

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
	s.InsertTask(&Task{
		ID: "task-1", SessionID: sessionID, Title: "Test", Status: "pending", MaxAttempts: 3,
	})

	now := time.Now()
	success := true
	attemptID, _ := s.RecordAttempt(&Attempt{
		TaskID: "task-1", SessionID: sessionID, Number: 1,
		AgentName: "claude", StartedAt: now, Success: &success,
	})

	s.SaveTranscript(attemptID, "Full agent output here...")
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
		s.RecordQuality(&QualitySnapshot{
			SessionID:   sessionID,
			Iteration:   i,
			OverallPass: pass,
			TestTotal:   10,
			TestPassed:  5 + i,
			TestFailed:  5 - i,
			FailedTestsJSON: `["test_a"]`,
		})
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

	s.RecordUsage(&ResourceUsage{
		SessionID: sessionID, Iteration: 1, ContainerTimeMs: 5000, EstimatedTokens: 1000,
	})
	s.RecordUsage(&ResourceUsage{
		SessionID: sessionID, Iteration: 2, ContainerTimeMs: 3000, EstimatedTokens: 800,
	})

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

	s.AddJournalEntry(&JournalEntry{
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
	})

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

	s.SaveSprintReport(&SprintReport{
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
	})

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

	s.SaveReviewResult(&ReviewResult{
		SessionID:    sessionID,
		Sprint:       1,
		ReviewAgent:  "claude-review",
		FindingsJSON: `[{"severity":"minor","description":"unused import"}]`,
		Summary:      "Looks good overall",
		Approved:     true,
	})

	// Verify via raw query (no dedicated getter needed for now)
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM review_results WHERE session_id = ?", sessionID).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 review result, got %d", count)
	}
}

func TestExportDashboardData(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("https://github.com/test/repo", "feat/test", "")

	s.InsertTask(&Task{
		ID: "task-1", SessionID: sessionID, Title: "Test", Status: "completed", MaxAttempts: 3,
	})
	s.InsertTask(&Task{
		ID: "task-2", SessionID: sessionID, Title: "Test 2", Status: "pending", MaxAttempts: 3,
	})

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
