package supervisor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swamp-dev/agentbox/internal/retro"
	"github.com/swamp-dev/agentbox/internal/store"
)

func TestApply_ReorderTasks(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, logger)

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
	s.InsertTask(&store.Task{
		ID: "t-1", SessionID: sessionID, Title: "Stuck task", Status: "pending", MaxAttempts: 3,
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

func TestApply_UpdateContext(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")
	s.InsertTask(&store.Task{
		ID: "t-1", SessionID: sessionID, Title: "Test",
		Status: "pending", MaxAttempts: 3, ContextNotes: "original",
	})

	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, logger)

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
	ac := NewAdaptiveController(s, sessionID, logger)

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
	ac := NewAdaptiveController(s, sessionID, logger)

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
	s.InsertTask(&store.Task{
		ID: "t-1", SessionID: sessionID, Title: "Skip me", Status: "pending", MaxAttempts: 3,
	})

	logger := testLogger()
	ac := NewAdaptiveController(s, sessionID, logger)

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
	ac := NewAdaptiveController(s, sessionID, logger)

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
	ac := NewAdaptiveController(s, sessionID, logger)

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
	ac := NewAdaptiveController(s, sessionID, logger)

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
