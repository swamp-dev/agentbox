package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/swamp-dev/agentbox/internal/agent"
	"github.com/swamp-dev/agentbox/internal/config"
	"github.com/swamp-dev/agentbox/internal/ralph"
)

var (
	ralphAgent         string
	ralphProject       string
	ralphMaxIterations int
	ralphPRDFile       string
	ralphAutoCommit    bool
)

var ralphCmd = &cobra.Command{
	Use:   "ralph",
	Short: "Run Ralph loop until PRD complete",
	Long: `Ralph implements the iterative agent execution pattern.

Each iteration:
1. Spawn fresh container with agent
2. Load PRD, find next incomplete task
3. Run agent with task-specific prompt
4. Check for completion signal
5. Run quality checks (typecheck, tests)
6. Commit changes to git
7. Update prd.json (mark task complete)
8. Append learnings to progress.txt
9. Repeat or exit

Examples:
  agentbox ralph --agent claude --max-iterations 10 --prd prd.json
  agentbox ralph --agent claude-cli  # Use subscription auth (no API key needed)
  agentbox ralph --auto-commit=false  # Don't auto-commit changes`,
	RunE: runRalph,
}

func init() {
	ralphCmd.Flags().StringVarP(&ralphAgent, "agent", "a", "claude", "agent to use (claude, claude-cli, amp, aider)")
	ralphCmd.Flags().StringVarP(&ralphProject, "project", "p", ".", "project directory")
	ralphCmd.Flags().IntVar(&ralphMaxIterations, "max-iterations", 10, "maximum iterations before stopping")
	ralphCmd.Flags().StringVar(&ralphPRDFile, "prd", "prd.json", "PRD file path")
	ralphCmd.Flags().BoolVar(&ralphAutoCommit, "auto-commit", true, "automatically commit changes after each task")
}

func runRalph(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// CLI flags override config only when explicitly set.
	if cmd.Flags().Changed("agent") {
		cfg.Agent.Name = ralphAgent
	}
	if cmd.Flags().Changed("max-iterations") {
		cfg.Ralph.MaxIterations = ralphMaxIterations
	}
	if cmd.Flags().Changed("prd") {
		cfg.Ralph.PRDFile = ralphPRDFile
	}
	if cmd.Flags().Changed("auto-commit") {
		cfg.Ralph.AutoCommit = ralphAutoCommit
	}

	// Resolve the effective agent name for validation below.
	ralphAgent = cfg.Agent.Name

	if err := agent.ValidateAPIKey(ralphAgent); err != nil {
		return err
	}

	// claude-cli requires network access for subscription auth
	if ralphAgent == "claude-cli" && cfg.Docker.Network == "none" {
		logger.Info("claude-cli requires network access, using restricted egress mode")
		cfg.Docker.Network = "restricted"
	}

	// Set agent-default endpoints for restricted mode.
	if cfg.Docker.Network == "restricted" && len(cfg.Docker.AllowedEndpoints) == 0 {
		ag, _ := agent.New(ralphAgent)
		if ag != nil {
			cfg.Docker.AllowedEndpoints = ag.AllowedEndpoints()
		}
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	prdPath := ralphProject + "/" + ralphPRDFile
	if _, err := os.Stat(prdPath); os.IsNotExist(err) {
		return fmt.Errorf("PRD file not found: %s\nRun 'agentbox init' to create one", prdPath)
	}

	loop, err := ralph.NewLoop(cfg, ralphProject, logger)
	if err != nil {
		return fmt.Errorf("initializing Ralph loop: %w", err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("received interrupt signal, stopping loop...")
		cancel()
	}()

	logger.Info("starting Ralph loop",
		"agent", ralphAgent,
		"project", ralphProject,
		"prd", ralphPRDFile,
		"max_iterations", ralphMaxIterations,
	)

	if err := loop.Run(ctx); err != nil {
		logger.Error("Ralph loop failed", "error", err)
		return err
	}

	status := loop.Status()
	fmt.Printf("\n%s\n", status.String())

	return nil
}
