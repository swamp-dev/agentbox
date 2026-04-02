package supervisor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swamp-dev/agentbox/internal/retro"
	"github.com/swamp-dev/agentbox/internal/store"
	"github.com/swamp-dev/agentbox/internal/taskdb"
)

func TestApply_ReorderTasks(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, nil, logger)

	recs := []retro.Recommendation{
		{Action: retro.RecReorderTasks, Description: "reorder for better flow"},
	}
	actions := ac.Apply(recs)
	if len(actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(actions))
	}
	if !strings.Contains(actions[0], "reorder tasks") {
		t.Errorf("expected reorder action, got %q", actions[0])
	}
}

func TestApply_DeferTask(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	if err := s.InsertTask(&store.Task{
		ID: "t-1", SessionID: sessionID, Title: "Stuck task", Status: "pending", MaxAttempts: 3,
	}); err != nil {
		t.Fatalf("InsertTask: %v", err)
	}

	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, nil, logger)

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

func TestApply_UpdateContext(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	if err := s.InsertTask(&store.Task{
		ID: "t-1", SessionID: sessionID, Title: "Test",
		Status: "pending", MaxAttempts: 3, ContextNotes: "original",
	}); err != nil {
		t.Fatalf("InsertTask: %v", err)
	}

	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, nil, logger)

	recs := []retro.Recommendation{
		{Action: retro.RecUpdateContext, TaskID: "t-1", Description: "compile errors on imports"},
	}
	actions := ac.Apply(recs)
	if len(actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(actions))
	}

	task, _ := s.GetTask("t-1")
	if !strings.Contains(task.ContextNotes, "original") {
		t.Error("expected original context notes to be preserved")
	}
	if !strings.Contains(task.ContextNotes, "compile errors on imports") {
		t.Error("expected updated context to include new info")
	}
	if !strings.Contains(task.ContextNotes, "[Retro") {
		t.Error("expected retro timestamp prefix in context notes")
	}
}

func TestApply_SwitchAgent(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, nil, logger)

	recs := []retro.Recommendation{
		{Action: retro.RecSwitchAgent, Description: "try fallback agent"},
	}
	actions := ac.Apply(recs)
	if len(actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(actions))
	}
	if !strings.Contains(actions[0], "switch agent") {
		t.Errorf("expected switch agent action, got %q", actions[0])
	}
}

func TestApply_UnknownAction(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, nil, logger)

	recs := []retro.Recommendation{
		{Action: "unknown_action", Description: "something weird"},
	}
	actions := ac.Apply(recs)
	// Unknown action should produce no actions (silently ignored)
	if len(actions) != 0 {
		t.Errorf("expected 0 actions for unknown action, got %d: %v", len(actions), actions)
	}
}

func TestApply_SkipTask(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	if err := s.InsertTask(&store.Task{
		ID: "t-1", SessionID: sessionID, Title: "Skip me", Status: "pending", MaxAttempts: 3,
	}); err != nil {
		t.Fatalf("InsertTask: %v", err)
	}

	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, nil, logger)

	recs := []retro.Recommendation{
		{Action: retro.RecSkipTask, TaskID: "t-1", Description: "Not relevant"},
	}
	actions := ac.Apply(recs)
	if len(actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(actions))
	}
	task, _ := s.GetTask("t-1")
	if task.Status != "deferred" {
		t.Errorf("expected task deferred after skip, got %s", task.Status)
	}
}

func TestApply_Escalate(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, nil, logger)

	recs := []retro.Recommendation{
		{Action: retro.RecEscalate, Description: "need human help"},
	}
	actions := ac.Apply(recs)
	if len(actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(actions))
	}
	if !strings.Contains(actions[0], "Escalation") {
		t.Errorf("expected escalation action, got %q", actions[0])
	}
}

func TestApply_Rollback(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, nil, logger)

	recs := []retro.Recommendation{
		{Action: retro.RecRollback, Description: "quality degrading"},
	}
	actions := ac.Apply(recs)
	if len(actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(actions))
	}
	if !strings.Contains(actions[0], "rollback") {
		t.Errorf("expected rollback action, got %q", actions[0])
	}
}

func TestWriteEscalation_CreatesFile(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, nil, logger)

	dir := t.TempDir()
	err := ac.WriteEscalation(dir, "first escalation")
	if err != nil {
		t.Fatalf("WriteEscalation: %v", err)
	}

	// Append a second one.
	err = ac.WriteEscalation(dir, "second escalation")
	if err != nil {
		t.Fatalf("WriteEscalation(2): %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".agentbox", "escalations.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "first escalation") {
		t.Error("expected first escalation message")
	}
	if !strings.Contains(content, "second escalation") {
		t.Error("expected second escalation message")
	}
}

func TestApply_ReorderTasks_DeprioritizesTask(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()

	tdb := taskdb.New()
	_ = tdb.Add(&taskdb.Task{ID: "t-1", Title: "Failing task", Status: taskdb.StatusPending, Priority: 3})
	_ = tdb.Add(&taskdb.Task{ID: "t-2", Title: "Other task", Status: taskdb.StatusPending, Priority: 5})

	ac := NewAdaptiveController(s, sessionID, tdb, logger)

	recs := []retro.Recommendation{
		{Action: retro.RecReorderTasks, TaskID: "t-1", Description: "repeatedly failing"},
	}
	actions := ac.Apply(recs)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if !strings.Contains(actions[0], "Deprioritized") {
		t.Errorf("expected deprioritized action, got %q", actions[0])
	}

	// The failing task should now have higher priority number (lower priority).
	task, _ := tdb.Get("t-1")
	if task.Priority <= 3 {
		t.Errorf("expected priority > 3 after deprioritization, got %d", task.Priority)
	}

	// t-2 should now be picked first by NextTask since it has lower priority number.
	next := tdb.NextTask()
	if next == nil || next.ID != "t-2" {
		t.Errorf("expected t-2 to be next task after reorder, got %v", next)
	}
}

func TestApply_ReorderTasks_EmptyTaskID(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()

	tdb := taskdb.New()
	ac := NewAdaptiveController(s, sessionID, tdb, logger)

	recs := []retro.Recommendation{
		{Action: retro.RecReorderTasks, TaskID: "", Description: "no specific task"},
	}
	actions := ac.Apply(recs)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	// Should fall back to recommendation message.
	if !strings.Contains(actions[0], "Recommendation: reorder tasks") {
		t.Errorf("expected fallback recommendation, got %q", actions[0])
	}
}

func TestApply_SplitTask_CreatesSubtasks(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()

	tdb := taskdb.New()
	_ = tdb.Add(&taskdb.Task{
		ID: "t-1", Title: "Big complex task", Description: "Do everything",
		Status: taskdb.StatusPending, Priority: 1, MaxAttempts: 3,
	})

	ac := NewAdaptiveController(s, sessionID, tdb, logger)

	recs := []retro.Recommendation{
		{Action: retro.RecSplitTask, TaskID: "t-1", Description: "too complex, break down"},
	}
	actions := ac.Apply(recs)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if !strings.Contains(actions[0], "Split task t-1") {
		t.Errorf("expected split action, got %q", actions[0])
	}

	// Parent should be deferred.
	parent, _ := tdb.Get("t-1")
	if parent.Status != taskdb.StatusDeferred {
		t.Errorf("expected parent deferred, got %s", parent.Status)
	}

	// Subtasks should exist.
	part1, ok1 := tdb.Get("t-1-part1")
	part2, ok2 := tdb.Get("t-1-part2")
	if !ok1 || !ok2 {
		t.Fatal("expected both subtasks to exist")
	}
	if part1.ParentID != "t-1" || part2.ParentID != "t-1" {
		t.Error("expected subtasks to reference parent")
	}
	if part1.Priority != 1 || part2.Priority != 1 {
		t.Error("expected subtasks to inherit parent priority")
	}
}

func TestApply_SplitTask_EmptyTaskID(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()

	tdb := taskdb.New()
	ac := NewAdaptiveController(s, sessionID, tdb, logger)

	recs := []retro.Recommendation{
		{Action: retro.RecSplitTask, TaskID: "", Description: "no specific task"},
	}
	actions := ac.Apply(recs)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if !strings.Contains(actions[0], "Recommendation: split task") {
		t.Errorf("expected fallback recommendation, got %q", actions[0])
	}
}

func TestApply_SplitTask_NonexistentTask(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()

	tdb := taskdb.New()
	ac := NewAdaptiveController(s, sessionID, tdb, logger)

	recs := []retro.Recommendation{
		{Action: retro.RecSplitTask, TaskID: "nonexistent", Description: "task not found"},
	}
	actions := ac.Apply(recs)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	// Should fall back to recommendation.
	if !strings.Contains(actions[0], "Recommendation: split task") {
		t.Errorf("expected fallback recommendation, got %q", actions[0])
	}
}

func TestApply_NilTaskDB_FallsBack(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()

	// nil taskDB — should produce recommendation-only actions.
	ac := NewAdaptiveController(s, sessionID, nil, logger)

	recs := []retro.Recommendation{
		{Action: retro.RecReorderTasks, TaskID: "t-1", Description: "reorder"},
		{Action: retro.RecSplitTask, TaskID: "t-1", Description: "split"},
	}
	actions := ac.Apply(recs)
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}
	if !strings.Contains(actions[0], "Recommendation: reorder tasks") {
		t.Errorf("expected reorder recommendation, got %q", actions[0])
	}
	if !strings.Contains(actions[1], "Recommendation: split task") {
		t.Errorf("expected split recommendation, got %q", actions[1])
	}
}
