package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/swamp-dev/agentbox/internal/metrics"
)

func TestSprintCmd_FlagRegistration(t *testing.T) {
	flags := []string{
		"repo",
		"prd",
		"agent",
		"review-agent",
		"sprint-size",
		"max-sprints",
		"budget-duration",
		"no-journal",
		"no-review",
		"dry-run",
		"branch",
		"docker-image",
		"docker-memory",
		"docker-cpus",
		"docker-network",
		"allow-endpoint",
		"resume",
		"session",
	}

	for _, name := range flags {
		t.Run(name, func(t *testing.T) {
			fl := sprintCmd.Flags().Lookup(name)
			if fl == nil {
				t.Errorf("flag %q not registered on sprint command", name)
			}
		})
	}
}

func TestSprintCmd_DefaultFlagValues(t *testing.T) {
	tests := []struct {
		flag string
		want string
	}{
		{"prd", "prd.json"},
		{"agent", "claude"},
		{"review-agent", "claude"},
		{"sprint-size", "5"},
		{"max-sprints", "20"},
		{"budget-duration", "8h"},
		{"docker-image", "full"},
		{"docker-memory", "4g"},
		{"docker-cpus", "2"},
		{"docker-network", "none"},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			fl := sprintCmd.Flags().Lookup(tt.flag)
			if fl == nil {
				t.Fatalf("flag %q not found", tt.flag)
			}
			if fl.DefValue != tt.want {
				t.Errorf("flag %q default = %q, want %q", tt.flag, fl.DefValue, tt.want)
			}
		})
	}
}

// resetSprintFlags resets package-level sprint flag variables to their defaults.
// This is necessary because cobra sets these globals during Execute() and they
// persist across test invocations on the shared rootCmd.
func resetSprintFlags() {
	sprintSessionID = 0
	sprintResume = false
	sprintDryRun = false
	sprintRepo = ""
	sprintPRD = "prd.json"
	sprintAgent = "claude"
	sprintReviewAgent = "claude"
	sprintSize = 5
	sprintMaxSprints = 20
	sprintBudgetDuration = "8h"
	sprintNoJournal = false
	sprintNoReview = false
	sprintBranch = ""
	sprintDockerImage = "full"
	sprintDockerMemory = "4g"
	sprintDockerCPUs = "2"
	sprintDockerNetwork = "none"
	sprintDockerAllowEndpoints = nil
}

func TestSprintCmd_SessionWithoutResume(t *testing.T) {
	resetSprintFlags()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"sprint", "--session", "42"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --session used without --resume, got nil")
	}

	if !strings.Contains(err.Error(), "--session requires --resume") {
		t.Errorf("expected '--session requires --resume' error, got: %v", err)
	}
}

func TestSprintCmd_InvalidBudgetDuration(t *testing.T) {
	resetSprintFlags()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"sprint", "--budget-duration", "not-a-duration"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid budget duration, got nil")
	}

	if !strings.Contains(err.Error(), "invalid budget duration") {
		t.Errorf("expected budget duration error, got: %v", err)
	}
}

func TestSprintCmd_DryRunWithMissingPRD(t *testing.T) {
	resetSprintFlags()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{
		"sprint",
		"--dry-run",
		"--prd", "/nonexistent/prd.json",
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for dry-run with missing PRD, got nil")
	}
}

func TestSprintCmd_DryRunWithInvalidRepoPath(t *testing.T) {
	resetSprintFlags()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{
		"sprint",
		"--dry-run",
		"--prd", "prd.json",
	})

	// This will try to find prd.json in cwd; the specific error depends on
	// the environment but it should not panic.
	_ = rootCmd.Execute()
}

func TestBudgetSummary(t *testing.T) {
	tests := []struct {
		name     string
		duration string
		tokens   int
		iters    int
		want     string
	}{
		{"unlimited", "", 0, 0, "unlimited"},
		{"duration only", "8h", 0, 0, "duration=8h0m0s"},
		{"tokens only", "", 100000, 0, "tokens=100000"},
		{"iterations only", "", 0, 50, "iterations=50"},
		{"all set", "4h", 50000, 20, "duration=4h0m0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b metrics.Budget
			if tt.duration != "" {
				d, err := time.ParseDuration(tt.duration)
				if err != nil {
					t.Fatal(err)
				}
				b.MaxDuration = d
			}
			b.MaxTokens = tt.tokens
			b.MaxIterations = tt.iters

			got := budgetSummary(b)
			if !strings.Contains(got, tt.want) {
				t.Errorf("budgetSummary() = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}

func TestOrDefault(t *testing.T) {
	tests := []struct {
		name string
		s    string
		def  string
		want string
	}{
		{"empty returns default", "", "fallback", "fallback"},
		{"non-empty returns value", "value", "fallback", "value"},
		{"both empty", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := orDefault(tt.s, tt.def)
			if got != tt.want {
				t.Errorf("orDefault(%q, %q) = %q, want %q", tt.s, tt.def, got, tt.want)
			}
		})
	}
}
