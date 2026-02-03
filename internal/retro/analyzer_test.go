package retro

import (
	"testing"
	"time"

	"github.com/swamp-dev/agentbox/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("opening test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAnalyze_BasicReport(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	// Insert tasks.
	s.InsertTask(&store.Task{
		ID: "t-1", SessionID: sessionID, Title: "Task 1", Status: "completed", MaxAttempts: 3,
	})
	s.InsertTask(&store.Task{
		ID: "t-2", SessionID: sessionID, Title: "Task 2", Status: "pending", MaxAttempts: 3,
	})

	// Record attempts.
	now := time.Now()
	success := true
	s.RecordAttempt(&store.Attempt{
		TaskID: "t-1", SessionID: sessionID, Number: 1,
		AgentName: "claude", StartedAt: now, Success: &success,
	})
	fail := false
	s.RecordAttempt(&store.Attempt{
		TaskID: "t-2", SessionID: sessionID, Number: 2,
		AgentName: "claude", StartedAt: now, Success: &fail, ErrorMsg: "compile error",
	})

	analyzer := NewAnalyzer(s, sessionID)
	report, err := analyzer.Analyze(1, 1, 5)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if report.SprintNumber != 1 {
		t.Errorf("expected sprint 1, got %d", report.SprintNumber)
	}
	if report.TasksAttempted != 2 {
		t.Errorf("expected 2 attempted, got %d", report.TasksAttempted)
	}
	if report.TasksCompleted != 1 {
		t.Errorf("expected 1 completed, got %d", report.TasksCompleted)
	}
}

func TestDetectPatterns_RepeatedFailure(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	s.InsertTask(&store.Task{
		ID: "t-1", SessionID: sessionID, Title: "Flaky task", Status: "pending", MaxAttempts: 5,
	})

	now := time.Now()
	fail := false
	for i := 1; i <= 3; i++ {
		s.RecordAttempt(&store.Attempt{
			TaskID: "t-1", SessionID: sessionID, Number: i,
			AgentName: "claude", StartedAt: now, Success: &fail, ErrorMsg: "test failure",
		})
	}

	analyzer := NewAnalyzer(s, sessionID)
	report, _ := analyzer.Analyze(1, 1, 5)

	found := false
	for _, p := range report.Patterns {
		if p.Type == PatternRepeatedFailure {
			found = true
		}
	}
	if !found {
		t.Error("expected repeated_failure pattern")
	}

	// Should recommend deferral for high severity.
	hasDefer := false
	for _, r := range report.Recommendations {
		if r.Action == RecDeferTask {
			hasDefer = true
		}
	}
	if !hasDefer {
		t.Error("expected defer_task recommendation")
	}
}

func TestSaveReport(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	analyzer := NewAnalyzer(s, sessionID)
	report := &SprintReport{
		SprintNumber:   1,
		StartIteration: 1,
		EndIteration:   5,
		TasksAttempted: 3,
		TasksCompleted: 2,
		Velocity:       0.67,
	}

	if err := analyzer.SaveReport(report); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}

	reports, _ := s.SprintReports(sessionID)
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
}

func TestSeverityFromFailCount(t *testing.T) {
	tests := []struct {
		count int
		want  string
	}{
		{1, "low"},
		{2, "medium"},
		{3, "high"},
		{5, "high"},
	}
	for _, tt := range tests {
		got := severityFromFailCount(tt.count)
		if got != tt.want {
			t.Errorf("severityFromFailCount(%d) = %q, want %q", tt.count, got, tt.want)
		}
	}
}
