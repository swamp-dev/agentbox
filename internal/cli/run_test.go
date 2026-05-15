package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// resetRunFlags resets package-level run flag variables to their defaults
// and clears cobra's Changed tracking. This is necessary because cobra sets
// these globals during Execute() and they persist across test invocations
// on the shared rootCmd.
func resetRunFlags() {
	runAgent = "claude"
	runProject = "."
	runPrompt = ""
	runNetwork = "none"
	runImage = "full"
	runInteractive = false
	runAllowNetwork = false
	runAllowEndpoints = nil

	runCmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
	})
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
	})
	verbose = false
	cfgFile = ""
}

func TestRunCmd_FlagRegistration(t *testing.T) {
	// Verify all expected flags are registered on the run command.
	flags := []struct {
		name      string
		shorthand string
	}{
		{"agent", "a"},
		{"project", "p"},
		{"prompt", ""},
		{"network", ""},
		{"image", ""},
		{"interactive", "i"},
		{"allow-network", ""},
		{"allow-endpoint", ""},
	}

	for _, f := range flags {
		t.Run(f.name, func(t *testing.T) {
			fl := runCmd.Flags().Lookup(f.name)
			if fl == nil {
				t.Errorf("flag %q not registered on run command", f.name)
				return
			}
			if f.shorthand != "" && fl.Shorthand != f.shorthand {
				t.Errorf("flag %q shorthand = %q, want %q", f.name, fl.Shorthand, f.shorthand)
			}
		})
	}
}

func TestRunCmd_DefaultFlagValues(t *testing.T) {
	tests := []struct {
		flag string
		want string
	}{
		{"agent", "claude"},
		{"project", "."},
		{"prompt", ""},
		{"network", "none"},
		{"image", "full"},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			fl := runCmd.Flags().Lookup(tt.flag)
			if fl == nil {
				t.Fatalf("flag %q not found", tt.flag)
			}
			if fl.DefValue != tt.want {
				t.Errorf("flag %q default = %q, want %q", tt.flag, fl.DefValue, tt.want)
			}
		})
	}
}

func TestRunCmd_MissingAPIKey(t *testing.T) {
	resetRunFlags()
	// Without ANTHROPIC_API_KEY set, run with default agent (claude) should fail.
	t.Setenv("ANTHROPIC_API_KEY", "")

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"run", "--project", "."})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing API key, got nil")
	}

	if !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Errorf("expected API key error, got: %v", err)
	}
}

func TestRunCmd_InvalidAgent(t *testing.T) {
	resetRunFlags()
	// An unknown agent name should fail config validation.
	// "bogus" passes ValidateAPIKey (no key check for unknown agents)
	// but fails config.Validate().
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"run", "--agent", "bogus", "--project", "."})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid agent, got nil")
	}

	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected invalid agent error, got: %v", err)
	}
}

func TestRunCmd_MutuallyExclusiveFlags(t *testing.T) {
	resetRunFlags()
	// --allow-network and --network are marked mutually exclusive.
	// Cobra rejects using both simultaneously.
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"run", "--allow-network", "--network", "bridge"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when using --allow-network and --network together, got nil")
	}

	// Cobra's error message varies by version; check for key fragments.
	errMsg := err.Error()
	if !strings.Contains(errMsg, "allow-network") || !strings.Contains(errMsg, "network") {
		t.Errorf("expected error about allow-network/network flag conflict, got: %v", err)
	}
}
