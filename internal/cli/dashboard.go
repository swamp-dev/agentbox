package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/swamp-dev/agentbox/internal/store"
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Show sprint progress and metrics",
	Long: `Display a dashboard of the current or most recent agentbox session,
including task status, quality trends, budget consumption, and recent journal entries.`,
	RunE: runDashboard,
}

var (
	dashboardJSON bool
	dashboardWatch bool
)

func init() {
	dashboardCmd.Flags().BoolVar(&dashboardJSON, "json", false, "output as JSON")
	dashboardCmd.Flags().BoolVar(&dashboardWatch, "watch", false, "continuously refresh")
}

func runDashboard(cmd *cobra.Command, args []string) error {
	s, sessionID, err := openLatestSession()
	if err != nil {
		return err
	}
	defer s.Close()

	data, err := s.ExportDashboardData(sessionID)
	if err != nil {
		return fmt.Errorf("loading dashboard data: %w", err)
	}

	if dashboardJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	}

	printDashboard(data)
	return nil
}

func printDashboard(data *store.DashboardData) {
	fmt.Println("=== Agentbox Dashboard ===")
	fmt.Println()

	// Session info.
	fmt.Printf("Session:  #%d (%s)\n", data.Session.ID, data.Session.Status)
	if data.Session.RepoURL != "" {
		fmt.Printf("Repo:     %s\n", data.Session.RepoURL)
	}
	if data.Session.BranchName != "" {
		fmt.Printf("Branch:   %s\n", data.Session.BranchName)
	}
	fmt.Printf("Started:  %s\n", data.Session.StartedAt.Format("2006-01-02 15:04:05"))
	fmt.Println()

	// Task stats.
	stats := data.TaskStats
	fmt.Println("--- Tasks ---")
	fmt.Printf("Total: %d | Completed: %d | Pending: %d | Failed: %d | Deferred: %d\n",
		stats.Total, stats.Completed, stats.Pending, stats.Failed, stats.Deferred)
	if stats.Total > 0 {
		pct := float64(stats.Completed) / float64(stats.Total) * 100
		fmt.Printf("Progress: %.1f%%\n", pct)
	}
	fmt.Println()

	// Quality.
	fmt.Println("--- Quality ---")
	fmt.Printf("Trend: %s | Test Pass Rate: %.1f%%\n", data.QualityTrend, data.TestPassRate*100)
	fmt.Println()

	// Resource usage.
	if data.TotalUsage != nil {
		fmt.Println("--- Resources ---")
		fmt.Printf("Iterations: %d | Tokens: %d | Container Time: %dms\n",
			data.TotalUsage.Iteration, data.TotalUsage.EstimatedTokens, data.TotalUsage.ContainerTimeMs)
		fmt.Println()
	}

	// Sprint reports.
	if len(data.SprintReports) > 0 {
		fmt.Println("--- Sprints ---")
		for _, r := range data.SprintReports {
			fmt.Printf("Sprint %d: %d/%d tasks (%.0f%% velocity) | Quality: %s\n",
				r.SprintNumber, r.TasksCompleted, r.TasksAttempted,
				r.Velocity*100, r.QualityTrend)
		}
		fmt.Println()
	}

	// Recent journal.
	if len(data.RecentJournal) > 0 {
		fmt.Println("--- Recent Journal ---")
		for _, e := range data.RecentJournal {
			fmt.Printf("[%s] %s: %s\n", e.Kind, e.Timestamp.Format("15:04"), e.Summary)
		}
		fmt.Println()
	}
}

func openLatestSession() (*store.Store, int64, error) {
	cwd, _ := os.Getwd()
	dbPath := filepath.Join(cwd, ".agentbox", "agentbox.db")

	s, err := store.Open(dbPath)
	if err != nil {
		return nil, 0, fmt.Errorf("no agentbox database found at %s: %w", dbPath, err)
	}

	sess, err := s.LatestSession()
	if err != nil {
		s.Close()
		return nil, 0, fmt.Errorf("no sessions found: %w", err)
	}

	return s, sess.ID, nil
}
