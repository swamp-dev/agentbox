package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/swamp-dev/agentbox/internal/mcp"
)

// Sentinel errors for wait exit codes.
var (
	ErrSessionFailed = errors.New("session failed")
	ErrWaitTimeout   = errors.New("wait timeout exceeded")
)

// waitConfig holds the parameters for runWait, extracted from cobra flags.
// Using a struct avoids reliance on mutable package-level vars in tests.
type waitConfig struct {
	session      string
	project      string
	timeout      time.Duration
	pollInterval time.Duration
	jsonOutput   bool
}

var waitCmd = &cobra.Command{
	Use:   "wait",
	Short: "Block until an async session completes",
	Long: `Wait blocks until the specified async session (started via ralph_start
or sprint_start) completes or fails, then prints the result.

Designed for use with Claude Code's Bash run_in_background to get
notified on completion without polling agentbox_status.

Exit codes:
  0  session completed successfully
  1  session failed
  2  timeout exceeded`,
	Example: `  agentbox wait --session abc-123 --project /path/to/project
  agentbox wait --session abc-123 --project . --json
  agentbox wait --session abc-123 --project . --timeout 1h`,
	RunE: func(cmd *cobra.Command, args []string) error {
		session, _ := cmd.Flags().GetString("session")
		project, _ := cmd.Flags().GetString("project")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		pollInterval, _ := cmd.Flags().GetDuration("poll-interval")
		jsonOutput, _ := cmd.Flags().GetBool("json")

		cfg := waitConfig{
			session:      session,
			project:      project,
			timeout:      timeout,
			pollInterval: pollInterval,
			jsonOutput:   jsonOutput,
		}
		return runWait(cfg)
	},
}

func init() {
	waitCmd.Flags().String("session", "", "session ID to wait for (required)")
	waitCmd.Flags().StringP("project", "p", ".", "project directory")
	waitCmd.Flags().Duration("timeout", 4*time.Hour, "maximum time to wait")
	waitCmd.Flags().Duration("poll-interval", 2*time.Second, "polling interval")
	waitCmd.Flags().Bool("json", false, "output result as JSON")
	_ = waitCmd.MarkFlagRequired("session")
}

func runWait(cfg waitConfig) error {
	deadline := time.Now().Add(cfg.timeout)

	for {
		state, err := mcp.ReadSessionState(cfg.project, cfg.session)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("reading session state: %w", err)
			}
			// File doesn't exist yet — session may not have started writing.
			if time.Now().After(deadline) {
				return ErrWaitTimeout
			}
			time.Sleep(cfg.pollInterval)
			continue
		}

		switch state.Status {
		case "completed":
			printWaitResult(state, cfg.jsonOutput)
			return nil
		case "failed":
			printWaitResult(state, cfg.jsonOutput)
			return ErrSessionFailed
		default:
			// Still running.
			if time.Now().After(deadline) {
				return ErrWaitTimeout
			}
			time.Sleep(cfg.pollInterval)
		}
	}
}

func printWaitResult(state *mcp.SessionState, jsonOutput bool) {
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(state); err != nil {
			fmt.Fprintf(os.Stderr, "error encoding result: %v\n", err)
		}
		return
	}

	fmt.Printf("Session:  %s\n", state.SessionID)
	fmt.Printf("Status:   %s\n", state.Status)
	if state.Error != "" {
		fmt.Printf("Error:    %s\n", state.Error)
	}
}
