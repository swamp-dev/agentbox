package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// resetRalphFlags resets package-level ralph flag variables to their defaults
// and clears cobra's Changed tracking. This is necessary because cobra sets
// these globals during Execute() and they persist across test invocations
// on the shared rootCmd.
func resetRalphFlags() {
	ralphAgent = "claude"
	ralphProject = "."
	ralphMaxIterations = 10
	ralphPRDFile = "prd.json"
	ralphAutoCommit = true

	ralphCmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
	})
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
	})
	verbose = false
	cfgFile = ""
}

func TestRalphCmd_FlagRegistration(t *testing.T) {
	flags := []struct {
		name      string
		shorthand string
	}{
		{"agent", "a"},
		{"project", "p"},
		{"max-iterations", ""},
		{"prd", ""},
		{"auto-commit", ""},
	}

	for _, f := range flags {
		t.Run(f.name, func(t *testing.T) {
			fl := ralphCmd.Flags().Lookup(f.name)
			if fl == nil {
				t.Errorf("flag %q not registered on ralph command", f.name)
				return
			}
			if f.shorthand != "" && fl.Shorthand != f.shorthand {
				t.Errorf("flag %q shorthand = %q, want %q", f.name, fl.Shorthand, f.shorthand)
			}
		})
	}
}

func TestRalphCmd_DefaultFlagValues(t *testing.T) {
	tests := []struct {
		flag string
		want string
	}{
		{"agent", "claude"},
		{"project", "."},
		{"max-iterations", "10"},
		{"prd", "prd.json"},
		{"auto-commit", "true"},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			fl := ralphCmd.Flags().Lookup(tt.flag)
			if fl == nil {
				t.Fatalf("flag %q not found", tt.flag)
			}
			if fl.DefValue != tt.want {
				t.Errorf("flag %q default = %q, want %q", tt.flag, fl.DefValue, tt.want)
			}
		})
	}
}

func TestRalphCmd_MissingAPIKey(t *testing.T) {
	resetRalphFlags()
	t.Setenv("ANTHROPIC_API_KEY", "")

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"ralph", "--project", "."})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing API key, got nil")
	}

	if !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Errorf("expected API key error, got: %v", err)
	}
}

func TestRalphCmd_InvalidAgent(t *testing.T) {
	resetRalphFlags()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"ralph", "--agent", "bogus"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid agent, got nil")
	}

	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected invalid agent error, got: %v", err)
	}
}

func TestRalphCmd_MissingPRDFile(t *testing.T) {
	resetRalphFlags()
	// Use claude-cli to bypass API key check (requires ~/.claude/ which exists in test env).
	dir := t.TempDir()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{
		"ralph",
		"--agent", "claude-cli",
		"--project", dir,
		"--prd", "nonexistent.json",
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing PRD file, got nil")
	}

	if !strings.Contains(err.Error(), "PRD file not found") {
		t.Errorf("expected PRD not found error, got: %v", err)
	}
}

func TestRalphCmd_PRDFileExistsButProjectMissing(t *testing.T) {
	resetRalphFlags()
	// When the project directory doesn't exist, the PRD path resolution
	// should fail because the combined path won't exist.
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{
		"ralph",
		"--agent", "claude-cli",
		"--project", "/nonexistent/project/path",
		"--prd", "prd.json",
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent project path, got nil")
	}

	if !strings.Contains(err.Error(), "PRD file not found") {
		t.Errorf("expected PRD not found error, got: %v", err)
	}
}

func TestRalphCmd_PRDPathCombination(t *testing.T) {
	resetRalphFlags()
	// Verify that the PRD path is constructed as project/prd.
	// We test this by checking that a PRD at project/custom.json is found
	// when --project=dir and --prd=custom.json.
	dir := t.TempDir()
	// Do NOT create the PRD file — we want the "not found" check to tell us
	// what path it tried to resolve.
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{
		"ralph",
		"--agent", "claude-cli",
		"--project", dir,
		"--prd", "custom.json",
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing PRD")
	}

	// The error message should reference the combined path.
	wantPath := dir + "/custom.json"
	if !strings.Contains(err.Error(), wantPath) {
		t.Errorf("expected error to reference %q, got: %v", wantPath, err)
	}
}
