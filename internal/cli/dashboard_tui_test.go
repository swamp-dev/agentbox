package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/swamp-dev/agentbox/internal/store"
)

func TestRenderWatchDashboard(t *testing.T) {
	now := time.Date(2026, 4, 8, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		data     *store.DashboardData
		width    int
		contains []string
	}{
		{
			name: "basic dashboard with all sections",
			data: &store.DashboardData{
				Session: &store.Session{
					ID:         1,
					Status:     "running",
					StartedAt:  now,
					RepoURL:    "https://github.com/example/repo",
					BranchName: "feat/dashboard",
				},
				TaskStats: &store.TaskStats{
					Total:      10,
					Completed:  5,
					InProgress: 2,
					Pending:    2,
					Failed:     1,
				},
				QualityTrend: "improving",
				TestPassRate: 0.85,
				TotalUsage: &store.ResourceUsage{
					Iteration:       8,
					EstimatedTokens: 50000,
					ContainerTimeMs: 120000,
				},
				RecentJournal: []*store.JournalEntry{
					{Kind: "COMPLETED", Timestamp: now, Summary: "Finished task-1"},
					{Kind: "STARTED", Timestamp: now, Summary: "Starting task-2"},
				},
			},
			width: 80,
			contains: []string{
				"Agentbox Dashboard",
				"Session #1",
				"running",
				"feat/dashboard",
				"https://github.com/example/repo",
				"50.0%",
				"✓ 5 completed",
				"▶ 2 in progress",
				"○ 2 pending",
				"✗ 1 failed",
				"improving",
				"85.0%",
				"Iterations: 8",
				"Tokens: 50000",
				"2m0s",
				"Finished task-1",
				"Starting task-2",
				"refreshed",
			},
		},
		{
			name: "minimal dashboard without optional sections",
			data: &store.DashboardData{
				Session: &store.Session{
					ID:        2,
					Status:    "completed",
					StartedAt: now,
				},
				TaskStats: &store.TaskStats{
					Total:     5,
					Completed: 5,
				},
				QualityTrend: "stable",
				TestPassRate: 1.0,
			},
			width: 60,
			contains: []string{
				"Session #2",
				"completed",
				"100.0%",
				"stable",
			},
		},
		{
			name: "dashboard with deferred tasks",
			data: &store.DashboardData{
				Session: &store.Session{
					ID:        3,
					Status:    "running",
					StartedAt: now,
				},
				TaskStats: &store.TaskStats{
					Total:    8,
					Deferred: 3,
					Pending:  5,
				},
				QualityTrend: "unknown",
				TestPassRate: 0,
			},
			width: 80,
			contains: []string{
				"⊘ 3 deferred",
				"0.0%",
			},
		},
		{
			name: "zero tasks",
			data: &store.DashboardData{
				Session: &store.Session{
					ID:        5,
					Status:    "running",
					StartedAt: now,
				},
				TaskStats:    &store.TaskStats{Total: 0},
				QualityTrend: "unknown",
				TestPassRate: 0,
			},
			width: 80,
			contains: []string{
				"Session #5",
				"0.0%",
				"✓ 0 completed",
				"▶ 0 in progress",
				"○ 0 pending",
				"✗ 0 failed",
			},
		},
		{
			name: "narrow width is clamped",
			data: &store.DashboardData{
				Session: &store.Session{
					ID:        4,
					Status:    "running",
					StartedAt: now,
				},
				TaskStats:    &store.TaskStats{Total: 1},
				QualityTrend: "stable",
				TestPassRate: 0.5,
			},
			width: 10, // below minimum
			contains: []string{
				"Agentbox Dashboard",
				"Session #4",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderWatchDashboard(tt.data, tt.width)

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("renderWatchDashboard() missing expected content %q\n\nFull output:\n%s", want, got)
				}
			}
		})
	}
}

func TestRenderWatchDashboard_NoResourceUsage(t *testing.T) {
	data := &store.DashboardData{
		Session: &store.Session{
			ID:        1,
			Status:    "running",
			StartedAt: time.Now(),
		},
		TaskStats:    &store.TaskStats{Total: 1},
		QualityTrend: "stable",
		TestPassRate: 0.5,
	}

	got := renderWatchDashboard(data, 80)
	if strings.Contains(got, "Resources") {
		t.Error("should not render Resources section when TotalUsage is nil")
	}
}

func TestRenderWatchDashboard_NoJournal(t *testing.T) {
	data := &store.DashboardData{
		Session: &store.Session{
			ID:        1,
			Status:    "running",
			StartedAt: time.Now(),
		},
		TaskStats:    &store.TaskStats{Total: 1},
		QualityTrend: "stable",
		TestPassRate: 0.5,
	}

	got := renderWatchDashboard(data, 80)
	if strings.Contains(got, "Recent Activity") {
		t.Error("should not render Recent Activity section when journal is empty")
	}
}

func TestRenderWatchDashboard_JournalLimitsFiveEntries(t *testing.T) {
	now := time.Now()
	entries := make([]*store.JournalEntry, 8)
	for i := range entries {
		entries[i] = &store.JournalEntry{
			Kind:      "COMPLETED",
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			Summary:   "entry-" + string(rune('A'+i)),
		}
	}

	data := &store.DashboardData{
		Session: &store.Session{
			ID:        1,
			Status:    "running",
			StartedAt: now,
		},
		TaskStats:     &store.TaskStats{Total: 1},
		QualityTrend:  "stable",
		TestPassRate:  0.5,
		RecentJournal: entries,
	}

	got := renderWatchDashboard(data, 80)

	// Should not contain first 3 entries (A, B, C).
	for _, excluded := range []string{"entry-A", "entry-B", "entry-C"} {
		if strings.Contains(got, excluded) {
			t.Errorf("should not contain old entry %q, only last 5", excluded)
		}
	}
	// Should contain last 5 (D through H).
	for _, included := range []string{"entry-D", "entry-E", "entry-F", "entry-G", "entry-H"} {
		if !strings.Contains(got, included) {
			t.Errorf("should contain recent entry %q", included)
		}
	}
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestClearScreen(t *testing.T) {
	var buf bytes.Buffer
	clearScreen(&buf)

	want := "\033[H\033[2J"
	if got := buf.String(); got != want {
		t.Errorf("clearScreen() wrote %q, want %q", got, want)
	}
}

func TestRunWatchMode_LoadsAndRenders(t *testing.T) {
	s := openTestStore(t)
	sessID, err := s.CreateSession("https://github.com/test/repo", "feat/test", "{}")
	if err != nil {
		t.Fatal(err)
	}

	data, err := s.ExportDashboardData(sessID)
	if err != nil {
		t.Fatalf("ExportDashboardData() error = %v", err)
	}

	output := renderWatchDashboard(data, 80)
	if !strings.Contains(output, "Agentbox Dashboard") {
		t.Error("rendered dashboard should contain header")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		ms   int
		want string
	}{
		{"milliseconds", 500, "500ms"},
		{"seconds", 5000, "5s"},
		{"minutes and seconds", 125000, "2m5s"},
		{"exact minutes", 120000, "2m0s"},
		{"zero", 0, "0ms"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.ms)
			if got != tt.want {
				t.Errorf("formatDuration(%d) = %q, want %q", tt.ms, got, tt.want)
			}
		})
	}
}
