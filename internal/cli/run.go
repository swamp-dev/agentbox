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
	"github.com/swamp-dev/agentbox/internal/container"
)

var (
	runAgent        string
	runProject      string
	runPrompt       string
	runNetwork      string
	runImage        string
	runInteractive  bool
	runAllowNetwork bool
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a single agent session in a container",
	Long: `Run starts a single AI agent session inside a Docker container.
The container is isolated from the host system by default.

Examples:
  agentbox run --agent claude --project ./my-app --prompt "Fix the bug in auth.ts"
  agentbox run --agent aider --interactive
  agentbox run --allow-network  # Enable network access for API calls`,
	RunE: runRun,
}

func init() {
	runCmd.Flags().StringVarP(&runAgent, "agent", "a", "claude", "agent to use (claude, amp, aider)")
	runCmd.Flags().StringVarP(&runProject, "project", "p", ".", "project directory to mount")
	runCmd.Flags().StringVar(&runPrompt, "prompt", "", "prompt to send to the agent")
	runCmd.Flags().StringVar(&runNetwork, "network", "none", "network mode (none, bridge, host)")
	runCmd.Flags().StringVar(&runImage, "image", "full", "Docker image to use")
	runCmd.Flags().BoolVarP(&runInteractive, "interactive", "i", false, "run in interactive mode")
	runCmd.Flags().BoolVar(&runAllowNetwork, "allow-network", false, "allow outbound network access")
}

func runRun(cmd *cobra.Command, args []string) error {
	if err := agent.ValidateAPIKey(runAgent); err != nil {
		return err
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	cfg.Agent.Name = runAgent
	cfg.Docker.Image = runImage
	if runAllowNetwork {
		cfg.Docker.Network = "bridge"
	} else {
		cfg.Docker.Network = runNetwork
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	ag, err := agent.New(runAgent)
	if err != nil {
		return err
	}

	cm, err := container.NewManager()
	if err != nil {
		return fmt.Errorf("creating container manager: %w", err)
	}
	defer cm.Close()

	agentCmd := ag.Command(runPrompt)
	env := ag.Environment()

	containerCfg, err := container.ConfigToContainerConfig(cfg, runProject, agentCmd, env)
	if err != nil {
		return fmt.Errorf("building container config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("received interrupt signal, stopping container...")
		cancel()
	}()

	logger.Info("starting container",
		"agent", runAgent,
		"project", runProject,
		"image", containerCfg.Image,
		"network", cfg.Docker.Network,
	)

	if runInteractive {
		containerID, err := cm.Create(ctx, containerCfg)
		if err != nil {
			return fmt.Errorf("creating container: %w", err)
		}
		defer func() { _ = cm.Remove(context.Background(), containerID) }()

		logger.Info("attaching to container", "id", containerID[:12])
		return cm.Attach(ctx, containerID)
	}

	output, err := cm.Run(ctx, containerCfg)
	if err != nil {
		logger.Error("agent execution failed", "error", err)
		if output != "" {
			fmt.Println("\n--- Agent Output ---")
			fmt.Println(output)
		}
		return err
	}

	fmt.Println(output)

	result := ag.ParseOutput(output)
	if result.Completed {
		logger.Info("agent completed task successfully")
	}

	return nil
}
