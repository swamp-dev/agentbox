package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/swamp-dev/agentbox/internal/metrics"
	"github.com/swamp-dev/agentbox/internal/ralph"
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
	sprintRepo                 string
	sprintPRD                  string
	sprintAgent                string
	sprintReviewAgent          string
	sprintSize                 int
	sprintMaxSprints           int
	sprintBudgetDuration       string
	sprintNoJournal            bool
	sprintNoReview             bool
	sprintDryRun               bool
	sprintBranch               string
	sprintDockerImage          string
	sprintDockerMemory         string
	sprintDockerCPUs           string
	sprintDockerNetwork        string
	sprintDockerAllowEndpoints []string
	sprintResume               bool
	sprintSessionID            int64
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
	sprintCmd.Flags().StringVar(&sprintDockerImage, "docker-image", "full", "Docker image (node, python, go, rust, full)")
	sprintCmd.Flags().StringVar(&sprintDockerMemory, "docker-memory", "4g", "container memory limit")
	sprintCmd.Flags().StringVar(&sprintDockerCPUs, "docker-cpus", "2", "container CPU limit")
	sprintCmd.Flags().StringVar(&sprintDockerNetwork, "docker-network", "none", "container network mode (none, bridge, host, restricted)")
	sprintCmd.Flags().StringSliceVar(&sprintDockerAllowEndpoints, "allow-endpoint", nil, "additional allowed endpoints for restricted mode (host:port)")
	sprintCmd.Flags().BoolVar(&sprintResume, "resume", false, "resume the most recent interrupted sprint session")
	sprintCmd.Flags().Int64Var(&sprintSessionID, "session", 0, "session ID to resume (used with --resume)")
}

func runSprint(cmd *cobra.Command, args []string) error {
	// Validate: --session requires --resume.
	if sprintSessionID != 0 && !sprintResume {
		return fmt.Errorf("--session requires --resume")
	}

	// Handle resume mode.
	if sprintResume {
		return runResume(cmd)
	}

	cfg := supervisor.DefaultConfig()

	// Only override defaults when CLI flags are explicitly set.
	if cmd.Flags().Changed("repo") {
		cfg.RepoURL = sprintRepo
	}
	if cmd.Flags().Changed("prd") {
		cfg.PRDFile = sprintPRD
	}
	if cmd.Flags().Changed("agent") {
		cfg.Agent = sprintAgent
	}
	if cmd.Flags().Changed("review-agent") {
		cfg.ReviewAgent = sprintReviewAgent
	}
	if cmd.Flags().Changed("sprint-size") {
		cfg.SprintSize = sprintSize
	}
	if cmd.Flags().Changed("max-sprints") {
		cfg.MaxSprints = sprintMaxSprints
	}
	if cmd.Flags().Changed("budget-duration") {
		cfg.BudgetDuration = sprintBudgetDuration
	}
	if cmd.Flags().Changed("no-journal") {
		cfg.JournalEnabled = !sprintNoJournal
	}
	if cmd.Flags().Changed("no-review") {
		cfg.ReviewEnabled = !sprintNoReview
	}
	if cmd.Flags().Changed("branch") {
		cfg.BranchName = sprintBranch
	}
	if cmd.Flags().Changed("docker-image") {
		cfg.DockerImage = sprintDockerImage
	}
	if cmd.Flags().Changed("docker-memory") {
		cfg.DockerMemory = sprintDockerMemory
	}
	if cmd.Flags().Changed("docker-cpus") {
		cfg.DockerCPUs = sprintDockerCPUs
	}
	if cmd.Flags().Changed("docker-network") {
		cfg.DockerNetwork = sprintDockerNetwork
	}
	if len(sprintDockerAllowEndpoints) > 0 {
		cfg.DockerAllowedEndpoints = sprintDockerAllowEndpoints
	}
	if cmd.Flags().Changed("dry-run") {
		cfg.DryRun = sprintDryRun
	}

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

func runResume(cmd *cobra.Command) error {
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

	// Determine which session to resume.
	var sessionID int64
	if cmd.Flags().Changed("session") && sprintSessionID > 0 {
		sessionID = sprintSessionID
	} else {
		// Find the most recent resumable session from the store.
		cwd, _ := os.Getwd()
		s, err := supervisor.FindResumableSession(cwd)
		if err != nil {
			return fmt.Errorf("finding resumable session: %w", err)
		}
		sessionID = s.ID
		logger.Info("found resumable session", "session_id", sessionID, "status", s.Status, "branch", s.BranchName)
	}

	sup, err := supervisor.NewForResume(sessionID, logger)
	if err != nil {
		return fmt.Errorf("initializing supervisor for resume: %w", err)
	}

	// Apply budget timeout, same as runSprint (S4).
	if sup.Config().Budget.MaxDuration > 0 {
		var budgetCancel context.CancelFunc
		ctx, budgetCancel = context.WithTimeout(ctx, sup.Config().Budget.MaxDuration)
		defer budgetCancel()
	}

	return sup.Resume(ctx)
}

func printDryRun(cfg *supervisor.Config) error {
	// Validate repo path or URL.
	if err := validateRepo(cfg); err != nil {
		return fmt.Errorf("repo validation: %w", err)
	}

	// Validate PRD file exists and is parseable.
	prd, err := validatePRD(cfg)
	if err != nil {
		return fmt.Errorf("PRD validation: %w", err)
	}

	fmt.Println("=== Agentbox Sprint (Dry Run) ===")
	fmt.Println()
	fmt.Printf("Repository:     %s\n", orDefault(cfg.RepoURL, "(current directory)"))
	fmt.Printf("PRD:            %s\n", cfg.PRDFile)
	fmt.Printf("Agent:          %s\n", cfg.Agent)
	fmt.Printf("Review Agent:   %s\n", cfg.ReviewAgent)
	fmt.Printf("Sprint Size:    %d iterations\n", cfg.SprintSize)
	fmt.Printf("Max Sprints:    %d\n", cfg.MaxSprints)
	fmt.Printf("Budget:         %s\n", budgetSummary(cfg.Budget))
	fmt.Printf("Docker Image:   %s\n", cfg.DockerImage)
	fmt.Printf("Docker Memory:  %s\n", cfg.DockerMemory)
	fmt.Printf("Docker CPUs:    %s\n", cfg.DockerCPUs)
	fmt.Printf("Docker Network: %s\n", cfg.DockerNetwork)
	fmt.Printf("Journal:        %v\n", cfg.JournalEnabled)
	fmt.Printf("Review:         %v\n", cfg.ReviewEnabled)
	fmt.Println()

	// Print task summary from validated PRD.
	tasks := prd.ExportTasks()
	fmt.Printf("PRD: %q — %d task(s)\n", prd.Name, len(tasks))
	for _, t := range tasks {
		deps := ""
		if len(t.DependsOn) > 0 {
			deps = fmt.Sprintf(" (depends on: %v)", t.DependsOn)
		}
		fmt.Printf("  • [%s] %s: %s%s\n", t.Status, t.ID, t.Title, deps)
	}
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

// validatePRD loads and parses the PRD file, returning the parsed PRD or an error.
func validatePRD(cfg *supervisor.Config) (*ralph.PRD, error) {
	prdPath := cfg.PRDFile
	if !filepath.IsAbs(prdPath) {
		workDir := cfg.WorkDir
		if workDir == "" {
			workDir = "."
		}
		prdPath = filepath.Join(workDir, prdPath)
	}

	prd, err := ralph.LoadPRD(prdPath)
	if err != nil {
		return nil, err
	}
	return prd, nil
}

// validateRepo checks that the repo path exists (if local) or that a remote
// URL is non-empty. Full URL validation is deferred to workflow.CloneOrOpen
// which handles all git-supported schemes including SCP-style SSH URLs
// (e.g. git@github.com:user/repo.git).
func validateRepo(cfg *supervisor.Config) error {
	if cfg.RepoURL != "" {
		// Remote repo: just verify non-empty. The workflow layer validates
		// the URL at clone time, supporting http(s), ssh, git, and SCP formats.
		return nil
	}

	// Local repo: check WorkDir exists.
	workDir := cfg.WorkDir
	if workDir == "" {
		return nil
	}

	info, err := os.Stat(workDir)
	if err != nil {
		return fmt.Errorf("work directory %q does not exist", workDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("work directory %q is not a directory", workDir)
	}
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
