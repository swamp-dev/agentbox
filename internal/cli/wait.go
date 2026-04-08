package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/swamp-dev/agentbox/internal/mcp"
)

var (
	waitSession      string
	waitProject      string
	waitTimeout      time.Duration
	waitPollInterval time.Duration
	waitJSON         bool
)

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
	RunE: runWait,
}

func init() {
	waitCmd.Flags().StringVar(&waitSession, "session", "", "session ID to wait for (required)")
	waitCmd.Flags().StringVarP(&waitProject, "project", "p", ".", "project directory")
	waitCmd.Flags().DurationVar(&waitTimeout, "timeout", 4*time.Hour, "maximum time to wait")
	waitCmd.Flags().DurationVar(&waitPollInterval, "poll-interval", 2*time.Second, "polling interval")
	waitCmd.Flags().BoolVar(&waitJSON, "json", false, "output result as JSON")
	_ = waitCmd.MarkFlagRequired("session")
}

func runWait(cmd *cobra.Command, args []string) error {
	deadline := time.Now().Add(waitTimeout)

	for {
		state, err := mcp.ReadSessionState(waitProject, waitSession)
		if err != nil {
			// File doesn't exist yet — session may not have started writing.
			if time.Now().After(deadline) {
				fmt.Fprintf(os.Stderr, "timeout: session %s did not complete within %s\n", waitSession, waitTimeout)
				os.Exit(2)
			}
			time.Sleep(waitPollInterval)
			continue
		}

		switch state.Status {
		case "completed":
			printWaitResult(state)
			return nil
		case "failed":
			printWaitResult(state)
			os.Exit(1)
		default:
			// Still running.
			if time.Now().After(deadline) {
				fmt.Fprintf(os.Stderr, "timeout: session %s did not complete within %s\n", waitSession, waitTimeout)
				os.Exit(2)
			}
			time.Sleep(waitPollInterval)
		}
	}
}

func printWaitResult(state *mcp.SessionState) {
	if waitJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(state)
		return
	}

	fmt.Printf("Session:  %s\n", state.SessionID)
	fmt.Printf("Status:   %s\n", state.Status)
	if state.Error != "" {
		fmt.Printf("Error:    %s\n", state.Error)
	}
}
