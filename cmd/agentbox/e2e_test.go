package main_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swamp-dev/agentbox/internal/ralph"
)

// setupGitRepo creates a temp directory with an initialized git repo and
// initial commit, suitable for running agentbox sprint --dry-run.
func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	for _, args := range [][]string{
		{"init", "--initial-branch", "main"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	// Create an initial commit so the repo has HEAD.
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "."},
		{"commit", "-m", "initial commit"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	return dir
}

// writePRD writes a PRD JSON file to the specified directory.
func writePRD(t *testing.T, dir string, prd ralph.PRD) string {
	t.Helper()
	data, err := json.MarshalIndent(prd, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "prd.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSprintDryRunE2E(t *testing.T) {
	if binaryPath == "" {
		t.Skip("binary not built (TestMain did not run)")
	}

	tests := []struct {
		name           string
		prd            ralph.PRD
		wantSubstrings []string
	}{
		{
			name: "two independent tasks",
			prd: ralph.PRD{
				Name:        "E2E Test Project",
				Description: "Test PRD",
				Tasks: []ralph.Task{
					{ID: "task-1", Title: "Set up CI pipeline", Description: "Configure GitHub Actions", Status: "pending", Priority: 1},
					{ID: "task-2", Title: "Add unit tests", Description: "Write tests for core module", Status: "pending", Priority: 2},
				},
			},
			wantSubstrings: []string{
				"Dry Run",
				"E2E Test Project",
				"2 task(s)",
				"Set up CI pipeline",
				"Add unit tests",
				"Execution plan",
				"task-1",
				"task-2",
			},
		},
		{
			name: "three tasks with dependencies",
			prd: ralph.PRD{
				Name:        "Dependency Test",
				Description: "Test PRD",
				Tasks: []ralph.Task{
					{ID: "t1", Title: "Foundation", Description: "Base layer", Status: "pending", Priority: 1},
					{ID: "t2", Title: "Feature A", Description: "Depends on foundation", Status: "pending", Priority: 2, DependsOn: []string{"t1"}},
					{ID: "t3", Title: "Feature B", Description: "Depends on A", Status: "pending", Priority: 3, DependsOn: []string{"t2"}},
				},
			},
			wantSubstrings: []string{
				"Dependency Test",
				"3 task(s)",
				"Foundation",
				"Feature A",
				"Feature B",
				"depends on",
				"Execution plan",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupGitRepo(t)
			writePRD(t, dir, tt.prd)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, binaryPath, "sprint", "--dry-run", "--repo", dir, "--prd", "prd.json")
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			output := string(out)

			if err != nil {
				t.Fatalf("sprint --dry-run failed: %v\n%s", err, output)
			}

			for _, want := range tt.wantSubstrings {
				if !strings.Contains(output, want) {
					t.Errorf("output missing %q\nGot:\n%s", want, output)
				}
			}
		})
	}
}

func TestSprintDryRunMissingPRD(t *testing.T) {
	if binaryPath == "" {
		t.Skip("binary not built (TestMain did not run)")
	}

	dir := setupGitRepo(t)
	// Deliberately do NOT write a prd.json.

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "sprint", "--dry-run", "--repo", dir, "--prd", "prd.json")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	output := string(out)

	if err == nil {
		t.Fatalf("expected sprint --dry-run to fail with missing PRD, got success:\n%s", output)
	}

	// Should mention the PRD problem.
	if !strings.Contains(strings.ToLower(output), "prd") {
		t.Errorf("error output should mention PRD, got:\n%s", output)
	}
}

func TestSprintDryRunInvalidPRD(t *testing.T) {
	if binaryPath == "" {
		t.Skip("binary not built (TestMain did not run)")
	}

	dir := setupGitRepo(t)

	// Write invalid JSON as prd.json.
	if err := os.WriteFile(filepath.Join(dir, "prd.json"), []byte("{not valid json}"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "sprint", "--dry-run", "--repo", dir, "--prd", "prd.json")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	output := string(out)

	if err == nil {
		t.Fatalf("expected sprint --dry-run to fail with invalid PRD, got success:\n%s", output)
	}

	if !strings.Contains(strings.ToLower(output), "prd") {
		t.Errorf("error output should mention PRD, got:\n%s", output)
	}
}
