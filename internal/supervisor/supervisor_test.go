package supervisor

import (
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/swamp-dev/agentbox/internal/metrics"
	"github.com/swamp-dev/agentbox/internal/retro"
	"github.com/swamp-dev/agentbox/internal/store"
	"github.com/swamp-dev/agentbox/internal/taskdb"
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
