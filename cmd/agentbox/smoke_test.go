package main_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// binaryPath is set by TestMain after building the binary.
var binaryPath string

func TestMain(m *testing.M) {
	// Build the binary to a temp location.
	tmp, err := os.MkdirTemp("", "agentbox-smoke-*")
	if err != nil {
		panic("failed to create temp dir: " + err.Error())
	}
	defer os.RemoveAll(tmp)

	binaryPath = filepath.Join(tmp, "agentbox")
	build := exec.Command("go", "build", "-o", binaryPath, "./")
	build.Dir = "." // cmd/agentbox
	if out, err := build.CombinedOutput(); err != nil {
		panic("failed to build binary: " + err.Error() + "\n" + string(out))
	}

	os.Exit(m.Run())
}

// runBinary executes the agentbox binary with the given args and a 5-second timeout.
func runBinary(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestVersion(t *testing.T) {
	out, err := runBinary(t, "version")
	if err != nil {
		t.Fatalf("version failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "agentbox") {
		t.Errorf("version output should contain 'agentbox', got: %s", out)
	}

	// Should contain structured version info fields.
	for _, field := range []string{"commit:", "built:", "go version:", "platform:"} {
		if !strings.Contains(out, field) {
			t.Errorf("version output missing %q, got: %s", field, out)
		}
	}
}

func TestRootHelp(t *testing.T) {
	out, err := runBinary(t, "--help")
	if err != nil {
		t.Fatalf("--help failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "agentbox") {
		t.Errorf("root help should mention 'agentbox', got: %s", out)
	}

	if !strings.Contains(out, "Available Commands") {
		t.Errorf("root help should list available commands, got: %s", out)
	}
}

func TestSubcommandHelp(t *testing.T) {
	commands := []string{
		"run",
		"ralph",
		"sprint",
		"init",
		"status",
		"images",
		"version",
		"dashboard",
		"retro",
		"journal",
		"mcp",
		"proxy",
	}

	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			out, err := runBinary(t, cmd, "--help")
			if err != nil {
				t.Fatalf("%s --help failed: %v\n%s", cmd, err, out)
			}

			// Every --help should produce some usage text.
			if len(out) == 0 {
				t.Errorf("%s --help produced no output", cmd)
			}
		})
	}
}

func TestRunRequiresProjectOrConfig(t *testing.T) {
	// Running 'run' without required flags in a directory without agentbox.yaml
	// should fail with a non-zero exit code.
	tmp := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "run")
	cmd.Dir = tmp
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected 'run' without args to fail, but it succeeded:\n%s", out)
	}

	// Should produce some error output (not just silently fail).
	if len(out) == 0 {
		t.Error("expected error output from 'run' without args, got nothing")
	}
}

func TestSprintDryRunValidation(t *testing.T) {
	// Sprint --dry-run in a directory without a PRD should fail with a clear error.
	tmp := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "sprint", "--dry-run", "--repo", tmp)
	cmd.Dir = tmp
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected 'sprint --dry-run' without PRD to fail, but it succeeded:\n%s", out)
	}

	if len(out) == 0 {
		t.Error("expected error output from sprint --dry-run, got nothing")
	}
}

func TestUnknownCommandFails(t *testing.T) {
	out, err := runBinary(t, "nonexistent-command")
	if err == nil {
		t.Fatalf("expected unknown command to fail, but it succeeded:\n%s", out)
	}

	if !strings.Contains(out, "unknown command") {
		t.Errorf("expected 'unknown command' in error, got: %s", out)
	}
}

func TestVerboseFlag(t *testing.T) {
	// The -v flag should not panic (regression test for a previous bug).
	out, err := runBinary(t, "-v", "version")
	if err != nil {
		t.Fatalf("-v version failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "agentbox") {
		t.Errorf("-v version should still print version info, got: %s", out)
	}
}
