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
)

// setupGitRepo creates a temp directory with an initialized git repo and
// initial commit, suitable for running agentbox sprint --dry-run.
func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	for _, args := range [][]string{
		{"init"},
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

// writePRD writes a PRD JSON file with the given tasks to the specified directory.
func writePRD(t *testing.T, dir string, name string, tasks []map[string]interface{}) string {
	t.Helper()
	prd := map[string]interface{}{
		"name":        name,
		"description": "Test PRD",
		"tasks":       tasks,
	}
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

// writeConfig writes a minimal agentbox.yaml to the directory.
func writeConfig(t *testing.T, dir string) {
	t.Helper()
	cfg := `version: "1.0"
project:
  name: test-project
  path: "."
agent:
  name: claude
docker:
  image: full
  resources:
    memory: "4g"
    cpus: "2"
  network: none
ralph:
  max_iterations: 10
  prd_file: prd.json
  auto_commit: true
  stop_signal: "<promise>COMPLETE</promise>"
`
	if err := os.WriteFile(filepath.Join(dir, "agentbox.yaml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSprintDryRunE2E(t *testing.T) {
	if binaryPath == "" {
		t.Skip("binary not built (TestMain did not run)")
	}

	tests := []struct {
		name           string
		tasks          []map[string]interface{}
		prdName        string
		wantSubstrings []string
	}{
		{
			name:    "two independent tasks",
			prdName: "E2E Test Project",
			tasks: []map[string]interface{}{
				{"id": "task-1", "title": "Set up CI pipeline", "description": "Configure GitHub Actions", "status": "pending", "priority": 1},
				{"id": "task-2", "title": "Add unit tests", "description": "Write tests for core module", "status": "pending", "priority": 2},
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
			name:    "three tasks with dependencies",
			prdName: "Dependency Test",
			tasks: []map[string]interface{}{
				{"id": "t1", "title": "Foundation", "description": "Base layer", "status": "pending", "priority": 1},
				{"id": "t2", "title": "Feature A", "description": "Depends on foundation", "status": "pending", "priority": 2, "depends_on": []string{"t1"}},
				{"id": "t3", "title": "Feature B", "description": "Depends on A", "status": "pending", "priority": 3, "depends_on": []string{"t2"}},
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
			writeConfig(t, dir)
			writePRD(t, dir, tt.prdName, tt.tasks)

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
	writeConfig(t, dir)
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
	writeConfig(t, dir)

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
