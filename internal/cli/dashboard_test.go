package cli

import (
	"testing"
	"time"

	"github.com/swamp-dev/agentbox/internal/store"
)

func TestPrintDashboard(t *testing.T) {
	tests := []struct {
		name string
		data *store.DashboardData
	}{
		{
			name: "basic dashboard",
			data: &store.DashboardData{
				Session: &store.Session{
					ID:         1,
					Status:     "running",
					StartedAt:  time.Now(),
					RepoURL:    "https://github.com/example/repo",
					BranchName: "feat/test",
				},
				TaskStats: &store.TaskStats{
					Total:     10,
					Completed: 5,
					Pending:   3,
					Failed:    2,
				},
				QualityTrend: "improving",
				TestPassRate: 0.85,
			},
		},
		{
			name: "dashboard with resource usage",
			data: &store.DashboardData{
				Session: &store.Session{
					ID:        2,
					Status:    "completed",
					StartedAt: time.Now(),
				},
				TaskStats: &store.TaskStats{
					Total:     5,
					Completed: 5,
				},
				TotalUsage: &store.ResourceUsage{
					Iteration:       10,
					EstimatedTokens: 50000,
					ContainerTimeMs: 120000,
				},
				QualityTrend: "stable",
				TestPassRate: 1.0,
			},
		},
		{
			name: "dashboard with sprints and journal",
			data: &store.DashboardData{
				Session: &store.Session{
					ID:        3,
					Status:    "running",
					StartedAt: time.Now(),
				},
				TaskStats: &store.TaskStats{
					Total: 0,
				},
				QualityTrend: "unknown",
				TestPassRate: 0,
				SprintReports: []*store.SprintReport{
					{
						SprintNumber:   1,
						TasksCompleted: 3,
						TasksAttempted: 4,
						Velocity:       0.75,
						QualityTrend:   "improving",
					},
				},
				RecentJournal: []*store.JournalEntry{
					{
						Kind:      "progress",
						Timestamp: time.Now(),
						Summary:   "Completed task-1",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// printDashboard only writes to stdout; verify it doesn't panic.
			printDashboard(tt.data)
		})
	}
}
