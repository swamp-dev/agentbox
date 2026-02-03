package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/swamp-dev/agentbox/internal/store"
)

var retroCmd = &cobra.Command{
	Use:   "retro",
	Short: "View sprint retrospective reports",
	Long:  `Display retrospective analysis from completed sprints, including detected patterns and recommendations.`,
	RunE:  runRetro,
}

var (
	retroSprint int
	retroLatest bool
	retroAll    bool
	retroExport bool
)

func init() {
	retroCmd.Flags().IntVar(&retroSprint, "sprint", 0, "show specific sprint number")
	retroCmd.Flags().BoolVar(&retroLatest, "latest", false, "show latest sprint only")
	retroCmd.Flags().BoolVar(&retroAll, "all", false, "show all sprints")
	retroCmd.Flags().BoolVar(&retroExport, "export", false, "export to .agentbox/retros/")
}

func runRetro(cmd *cobra.Command, args []string) error {
	s, sessionID, err := openLatestSession()
	if err != nil {
		return err
	}
	defer s.Close()

	reports, err := s.SprintReports(sessionID)
	if err != nil {
		return fmt.Errorf("loading sprint reports: %w", err)
	}

	if len(reports) == 0 {
		fmt.Println("No sprint reports found.")
		return nil
	}

	// Filter reports.
	if retroSprint > 0 {
		filtered := reports[:0]
		for _, r := range reports {
			if r.SprintNumber == retroSprint {
				filtered = append(filtered, r)
			}
		}
		reports = filtered
	} else if retroLatest {
		reports = reports[len(reports)-1:]
	}

	if retroExport {
		return exportRetros(reports)
	}

	for _, r := range reports {
		fmt.Printf("=== Sprint %d ===\n", r.SprintNumber)
		fmt.Printf("Iterations: %d-%d\n", r.StartIteration, r.EndIteration)
		fmt.Printf("Tasks: %d attempted, %d completed, %d failed\n",
			r.TasksAttempted, r.TasksCompleted, r.TasksFailed)
		fmt.Printf("Velocity: %.0f%% | Quality: %s | Pass Rate: %.1f%%\n",
			r.Velocity*100, r.QualityTrend, r.TestPassRate*100)
		fmt.Printf("Tokens: %d | Duration: %dms\n", r.TotalTokens, r.DurationMs)

		if r.PatternsJSON != "" && r.PatternsJSON != "null" {
			fmt.Println("\nPatterns:")
			fmt.Println(r.PatternsJSON)
		}
		if r.RecommendationsJSON != "" && r.RecommendationsJSON != "null" {
			fmt.Println("\nRecommendations:")
			fmt.Println(r.RecommendationsJSON)
		}
		fmt.Println()
	}

	return nil
}

func exportRetros(reports []*store.SprintReport) error {
	cwd, _ := os.Getwd()
	dir := cwd + "/.agentbox/retros"
	os.MkdirAll(dir, 0755)

	for _, r := range reports {
		path := fmt.Sprintf("%s/sprint-%d.json", dir, r.SprintNumber)
		data, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return err
		}
		fmt.Printf("Exported: %s\n", path)
	}
	return nil
}
