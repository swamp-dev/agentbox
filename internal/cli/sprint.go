package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/swamp-dev/agentbox/internal/metrics"
	"github.com/swamp-dev/agentbox/internal/supervisor"
)

var sprintCmd = &cobra.Command{
	Use:   "sprint",
	Short: "Run autonomous development sprint",
	Long: `Run an autonomous development sprint that decomposes a PRD into tasks,
executes them iteratively using AI agents in containers, runs quality checks,
performs code reviews, and opens a pull request with the results.

Leave it running overnight or over a weekend — come back to a PR with finished
code, retrospective reports, and a dev diary of the agent's experience.`,
	RunE: runSprint,
}

var (
	sprintRepo          string
	sprintPRD           string
	sprintAgent         string
	sprintReviewAgent   string
	sprintSize          int
	sprintMaxSprints    int
	sprintBudgetDuration string
	sprintNoJournal     bool
	sprintNoReview      bool
	sprintDryRun        bool
	sprintBranch        string
)

func init() {
	sprintCmd.Flags().StringVar(&sprintRepo, "repo", "", "repository URL to clone (or use current dir)")
	sprintCmd.Flags().StringVar(&sprintPRD, "prd", "prd.json", "PRD file path")
	sprintCmd.Flags().StringVar(&sprintAgent, "agent", "claude", "primary coding agent")
	sprintCmd.Flags().StringVar(&sprintReviewAgent, "review-agent", "claude", "review agent")
	sprintCmd.Flags().IntVar(&sprintSize, "sprint-size", 5, "iterations per sprint")
	sprintCmd.Flags().IntVar(&sprintMaxSprints, "max-sprints", 20, "maximum sprints")
	sprintCmd.Flags().StringVar(&sprintBudgetDuration, "budget-duration", "8h", "maximum runtime")
	sprintCmd.Flags().BoolVar(&sprintNoJournal, "no-journal", false, "disable journal entries")
	sprintCmd.Flags().BoolVar(&sprintNoReview, "no-review", false, "skip code review step")
	sprintCmd.Flags().BoolVar(&sprintDryRun, "dry-run", false, "show execution plan without running")
	sprintCmd.Flags().StringVar(&sprintBranch, "branch", "", "branch name (auto-generated if empty)")
}

func runSprint(cmd *cobra.Command, args []string) error {
	cfg := supervisor.DefaultConfig()
	cfg.RepoURL = sprintRepo
	cfg.PRDFile = sprintPRD
	cfg.Agent = sprintAgent
	cfg.ReviewAgent = sprintReviewAgent
	cfg.SprintSize = sprintSize
	cfg.MaxSprints = sprintMaxSprints
	cfg.BudgetDuration = sprintBudgetDuration
	cfg.JournalEnabled = !sprintNoJournal
	cfg.ReviewEnabled = !sprintNoReview
	cfg.BranchName = sprintBranch

	if err := cfg.ParseBudgetDuration(); err != nil {
		return fmt.Errorf("invalid budget duration: %w", err)
	}

	if cfg.RepoURL == "" {
		cwd, _ := os.Getwd()
		cfg.WorkDir = cwd
	}

	if sprintDryRun {
		return printDryRun(cfg)
	}

	// Set up signal handling.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("received shutdown signal, finishing current iteration...")
		cancel()
	}()

	// Set budget duration.
	if cfg.Budget.MaxDuration > 0 {
		budgetCtx, budgetCancel := context.WithTimeout(ctx, cfg.Budget.MaxDuration)
		defer budgetCancel()
		ctx = budgetCtx
	}

	sup, err := supervisor.New(cfg, logger)
	if err != nil {
		return fmt.Errorf("initializing supervisor: %w", err)
	}

	return sup.Run(ctx)
}

func printDryRun(cfg *supervisor.Config) error {
	fmt.Println("=== Agentbox Sprint (Dry Run) ===")
	fmt.Println()
	fmt.Printf("Repository:     %s\n", orDefault(cfg.RepoURL, "(current directory)"))
	fmt.Printf("PRD:            %s\n", cfg.PRDFile)
	fmt.Printf("Agent:          %s\n", cfg.Agent)
	fmt.Printf("Review Agent:   %s\n", cfg.ReviewAgent)
	fmt.Printf("Sprint Size:    %d iterations\n", cfg.SprintSize)
	fmt.Printf("Max Sprints:    %d\n", cfg.MaxSprints)
	fmt.Printf("Budget:         %s\n", budgetSummary(cfg.Budget))
	fmt.Printf("Journal:        %v\n", cfg.JournalEnabled)
	fmt.Printf("Review:         %v\n", cfg.ReviewEnabled)
	fmt.Println()
	fmt.Println("Execution plan:")
	fmt.Println("  1. Clone/open repository")
	fmt.Println("  2. Create worktree branch")
	fmt.Println("  3. Import PRD → task database")
	fmt.Printf("  4. Run up to %d sprints × %d iterations\n", cfg.MaxSprints, cfg.SprintSize)
	if cfg.ReviewEnabled {
		fmt.Printf("  5. Code review after each %s\n", cfg.ReviewAfter)
	}
	fmt.Println("  6. Open pull request")
	fmt.Println()
	fmt.Println("(No changes made — use without --dry-run to execute)")
	return nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func budgetSummary(b metrics.Budget) string {
	parts := []string{}
	if b.MaxDuration > 0 {
		parts = append(parts, fmt.Sprintf("duration=%s", b.MaxDuration))
	}
	if b.MaxTokens > 0 {
		parts = append(parts, fmt.Sprintf("tokens=%d", b.MaxTokens))
	}
	if b.MaxIterations > 0 {
		parts = append(parts, fmt.Sprintf("iterations=%d", b.MaxIterations))
	}
	if len(parts) == 0 {
		return "unlimited"
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}
