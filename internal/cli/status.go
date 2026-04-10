package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/swamp-dev/agentbox/internal/config"
	"github.com/swamp-dev/agentbox/internal/ralph"
)

var (
	statusProject string
	statusPRD     string
	statusJSON    bool
	statusTasks   bool
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Ralph loop progress",
	Long: `Status displays the current progress of the Ralph loop execution.

It shows:
- Overall completion percentage
- Task counts (completed, in progress, pending)
- Current/next task details
- Recent progress entries

Examples:
  agentbox status
  agentbox status --prd custom-prd.json
  agentbox status --json`,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().StringVarP(&statusProject, "project", "p", ".", "project directory")
	statusCmd.Flags().StringVar(&statusPRD, "prd", "prd.json", "PRD file path")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "output in JSON format")
	statusCmd.Flags().BoolVar(&statusTasks, "tasks", false, "show task list")
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		cfg = config.DefaultConfig()
	}

	prdPath := filepath.Join(statusProject, statusPRD)
	if _, err := os.Stat(prdPath); os.IsNotExist(err) {
		return fmt.Errorf("PRD file not found: %s\nRun 'agentbox init' to create one", prdPath)
	}

	prd, err := ralph.LoadPRD(prdPath)
	if err != nil {
		return fmt.Errorf("loading PRD: %w", err)
	}

	progressPath := filepath.Join(statusProject, cfg.Ralph.ProgressFile)
	progress := ralph.NewProgress(progressPath)
	if err := progress.Load(); err != nil {
		logger.Warn("could not load progress file", "error", err)
	}

	if statusJSON {
		return printStatusJSON(prd, progress, statusTasks)
	}

	if err := printStatusText(prd, progress); err != nil {
		return err
	}

	if statusTasks {
		printTaskList(prd)
	}

	return nil
}

func printStatusText(prd *ralph.PRD, progress *ralph.Progress) error {
	fmt.Printf("╔══════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║  AGENTBOX STATUS                                             ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  Project: %-51s ║\n", prd.Name)
	fmt.Printf("╠══════════════════════════════════════════════════════════════╣\n")

	progressBar := renderProgressBar(prd.Progress(), 40)
	fmt.Printf("║  Progress: %s %5.1f%% ║\n", progressBar, prd.Progress())
	fmt.Printf("║                                                              ║\n")
	fmt.Printf("║  Tasks:                                                      ║\n")
	fmt.Printf("║    ✓ Completed:   %3d                                        ║\n", prd.Metadata.Completed)
	fmt.Printf("║    ▶ In Progress: %3d                                        ║\n", prd.Metadata.InProgress)
	fmt.Printf("║    ○ Pending:     %3d                                        ║\n", prd.Metadata.Pending)
	fmt.Printf("║    ─────────────────                                         ║\n")
	fmt.Printf("║    Total:         %3d                                        ║\n", prd.Metadata.TotalTasks)
	fmt.Printf("╠══════════════════════════════════════════════════════════════╣\n")

	nextTask := prd.NextTask()
	if nextTask != nil {
		fmt.Printf("║  Next Task:                                                  ║\n")
		fmt.Printf("║    ID: %-55s ║\n", nextTask.ID)
		title := truncate(nextTask.Title, 53)
		fmt.Printf("║    Title: %-52s ║\n", title)
	} else if prd.IsComplete() {
		fmt.Printf("║  ✓ All tasks completed!                                      ║\n")
	} else {
		fmt.Printf("║  ⚠ No available tasks (check dependencies)                   ║\n")
	}

	fmt.Printf("╠══════════════════════════════════════════════════════════════╣\n")

	entries := progress.GetEntries()
	if len(entries) > 0 {
		fmt.Printf("║  Recent Activity:                                            ║\n")
		start := len(entries) - 5
		if start < 0 {
			start = 0
		}
		for _, e := range entries[start:] {
			status := statusIcon(e.Status)
			title := truncate(e.TaskTitle, 48)
			fmt.Printf("║    %s %-52s ║\n", status, title)
		}
	} else {
		fmt.Printf("║  No activity yet. Run 'agentbox ralph' to start.            ║\n")
	}

	fmt.Printf("╚══════════════════════════════════════════════════════════════╝\n")

	return nil
}

type statusJSONOutput struct {
	Project    string          `json:"project"`
	Progress   float64         `json:"progress"`
	Tasks      statusTaskCount `json:"tasks"`
	TaskList   []taskEntry     `json:"task_list,omitempty"`
	IsComplete bool            `json:"is_complete"`
}

type statusTaskCount struct {
	Total      int `json:"total"`
	Completed  int `json:"completed"`
	InProgress int `json:"in_progress"`
	Pending    int `json:"pending"`
}

type taskEntry struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

func printStatusJSON(prd *ralph.PRD, progress *ralph.Progress, includeTasks bool) error {
	out := statusJSONOutput{
		Project:  prd.Name,
		Progress: prd.Progress(),
		Tasks: statusTaskCount{
			Total:      prd.Metadata.TotalTasks,
			Completed:  prd.Metadata.Completed,
			InProgress: prd.Metadata.InProgress,
			Pending:    prd.Metadata.Pending,
		},
		IsComplete: prd.IsComplete(),
	}

	if includeTasks {
		out.TaskList = make([]taskEntry, len(prd.Tasks))
		for i, task := range prd.Tasks {
			out.TaskList[i] = taskEntry{
				ID:     task.ID,
				Title:  task.Title,
				Status: task.Status,
			}
		}
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling status JSON: %w", err)
	}
	fmt.Println(string(data))

	return nil
}

func printTaskList(prd *ralph.PRD) {
	fmt.Println("Tasks:")
	for _, task := range prd.Tasks {
		icon := statusIcon(task.Status)
		title := truncate(task.Title, 60)
		fmt.Printf("  %s %-10s %s\n", icon, task.ID, title)
	}
	fmt.Println()
}

func renderProgressBar(percent float64, width int) string {
	filled := int(percent / 100.0 * float64(width))
	if filled > width {
		filled = width
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return "[" + bar + "]"
}

func statusIcon(status string) string {
	switch status {
	case "COMPLETED", "done":
		return "✓"
	case "STARTED", "in_progress":
		return "▶"
	case "FAILED", "blocked":
		return "✗"
	case "ITERATION":
		return "↻"
	default:
		return "○"
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
