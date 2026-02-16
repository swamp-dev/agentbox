package metrics

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

func TestCollectorSummary(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	c := NewCollector(s, sessionID)

	if err := c.RecordUsage(&store.ResourceUsage{
		Iteration: 1, ContainerTimeMs: 5000, EstimatedTokens: 1000, AgentName: "claude",
	}); err != nil {
		t.Fatalf("RecordUsage: %v", err)
	}
	if err := c.RecordQuality(&store.QualitySnapshot{
		Iteration: 1, OverallPass: true, TestTotal: 10, TestPassed: 9, TestFailed: 1,
	}); err != nil {
		t.Fatalf("RecordQuality: %v", err)
	}

	summary, err := c.Summary()
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestBudgetEnforcer_NotExceeded(t *testing.T) {
	budget := Budget{
		MaxTokens:     100000,
		MaxDuration:   1 * time.Hour,
		MaxIterations: 50,
		WarnThreshold: 0.8,
	}
	enforcer := NewBudgetEnforcer(budget)
	status := enforcer.Check(1000, 5)

	if status.Exceeded {
		t.Error("should not be exceeded")
	}
	if status.Warning {
		t.Error("should not be warning")
	}
}

func TestBudgetEnforcer_TokenExceeded(t *testing.T) {
	budget := Budget{MaxTokens: 1000, WarnThreshold: 0.8}
	enforcer := NewBudgetEnforcer(budget)
	status := enforcer.Check(1001, 0)

	if !status.Exceeded {
		t.Error("should be exceeded")
	}
}

func TestBudgetEnforcer_Warning(t *testing.T) {
	budget := Budget{MaxTokens: 1000, WarnThreshold: 0.8}
	enforcer := NewBudgetEnforcer(budget)
	status := enforcer.Check(850, 0)

	if status.Exceeded {
		t.Error("should not be exceeded")
	}
	if !status.Warning {
		t.Error("should be warning")
	}
}

func TestBudgetEnforcer_IterationExceeded(t *testing.T) {
	budget := Budget{MaxIterations: 10, WarnThreshold: 0.8}
	enforcer := NewBudgetEnforcer(budget)
	status := enforcer.Check(0, 10)

	if !status.Exceeded {
		t.Error("should be exceeded")
	}
}

func TestParseGoTestOutput(t *testing.T) {
	output := `=== RUN   TestAdd
--- PASS: TestAdd (0.00s)
=== RUN   TestSubtract
--- FAIL: TestSubtract (0.00s)
    math_test.go:15: expected 0, got 1
=== RUN   TestMultiply
--- SKIP: TestMultiply (0.00s)
FAIL
`
	stats := ParseGoTestOutput(output)
	if stats.Total != 3 {
		t.Errorf("expected 3 total, got %d", stats.Total)
	}
	if stats.Passed != 1 {
		t.Errorf("expected 1 passed, got %d", stats.Passed)
	}
	if stats.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", stats.Failed)
	}
	if stats.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", stats.Skipped)
	}
	if len(stats.FailedTests) != 1 || stats.FailedTests[0] != "TestSubtract" {
		t.Errorf("expected [TestSubtract], got %v", stats.FailedTests)
	}
}

func TestParseGoTestOutput_SummaryOnly(t *testing.T) {
	output := `ok  	github.com/user/pkg	0.015s
FAIL	github.com/user/pkg2	0.023s
ok  	github.com/user/pkg3	0.005s
`
	stats := ParseGoTestOutput(output)
	if stats.Total != 3 {
		t.Errorf("expected 3 total, got %d", stats.Total)
	}
	if stats.Passed != 2 {
		t.Errorf("expected 2 passed, got %d", stats.Passed)
	}
	if stats.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", stats.Failed)
	}
}

func TestParseJestOutput(t *testing.T) {
	output := `PASS src/utils.test.ts
FAIL src/auth.test.ts
  ● login > should authenticate user
  ● login > should reject invalid password

Tests:  2 failed, 1 skipped, 5 passed, 8 total
`
	stats := ParseJestOutput(output)
	if stats.Total != 8 {
		t.Errorf("expected 8 total, got %d", stats.Total)
	}
	if stats.Passed != 5 {
		t.Errorf("expected 5 passed, got %d", stats.Passed)
	}
	if stats.Failed != 2 {
		t.Errorf("expected 2 failed, got %d", stats.Failed)
	}
	if stats.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", stats.Skipped)
	}
}

func TestParseGenericTestOutput(t *testing.T) {
	output := `PASS test_add
PASS test_subtract
FAIL test_divide
ok test_multiply
`
	stats := ParseGenericTestOutput(output)
	if stats.Total != 4 {
		t.Errorf("expected 4 total, got %d", stats.Total)
	}
	if stats.Passed != 3 {
		t.Errorf("expected 3 passed, got %d", stats.Passed)
	}
	if stats.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", stats.Failed)
	}
}

func TestTestStatsPassRate(t *testing.T) {
	stats := &TestStats{Total: 10, Passed: 8}
	rate := stats.PassRate()
	if rate != 0.8 {
		t.Errorf("expected 0.8, got %f", rate)
	}

	empty := &TestStats{}
	if empty.PassRate() != 0 {
		t.Errorf("expected 0 for empty stats")
	}
}
