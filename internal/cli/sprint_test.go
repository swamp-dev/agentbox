package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swamp-dev/agentbox/internal/supervisor"
)

func TestPrintDryRun_ValidPRD(t *testing.T) {
	dir := t.TempDir()
	prdPath := filepath.Join(dir, "prd.json")

	prdContent := `{
		"name": "test-project",
		"description": "A test PRD",
		"tasks": [
			{"id": "task-1", "title": "Setup", "status": "pending", "priority": 1},
			{"id": "task-2", "title": "Implement", "status": "pending", "priority": 2, "depends_on": ["task-1"]},
			{"id": "task-3", "title": "Test", "status": "pending", "priority": 3, "depends_on": ["task-2"]}
		]
	}`
	if err := os.WriteFile(prdPath, []byte(prdContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := supervisor.DefaultConfig()
	cfg.PRDFile = prdPath
	cfg.WorkDir = dir

	err := printDryRun(cfg)
	if err != nil {
		t.Errorf("printDryRun() with valid PRD should not error, got: %v", err)
	}
}

func TestPrintDryRun_MissingPRD(t *testing.T) {
	cfg := supervisor.DefaultConfig()
	cfg.PRDFile = "/nonexistent/path/prd.json"
	cfg.WorkDir = t.TempDir()

	err := printDryRun(cfg)
	if err == nil {
		t.Error("printDryRun() with missing PRD should return error")
	}
}

func TestPrintDryRun_MalformedPRD(t *testing.T) {
	dir := t.TempDir()
	prdPath := filepath.Join(dir, "prd.json")

	if err := os.WriteFile(prdPath, []byte("not valid json {{{"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := supervisor.DefaultConfig()
	cfg.PRDFile = prdPath
	cfg.WorkDir = dir

	err := printDryRun(cfg)
	if err == nil {
		t.Error("printDryRun() with malformed PRD should return error")
	}
}

func TestPrintDryRun_RelativePRDPath(t *testing.T) {
	dir := t.TempDir()
	prdPath := filepath.Join(dir, "prd.json")

	prdContent := `{
		"name": "relative-test",
		"tasks": [
			{"id": "task-1", "title": "Only task", "status": "pending"}
		]
	}`
	if err := os.WriteFile(prdPath, []byte(prdContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := supervisor.DefaultConfig()
	cfg.PRDFile = "prd.json" // relative path
	cfg.WorkDir = dir

	err := printDryRun(cfg)
	if err != nil {
		t.Errorf("printDryRun() with relative PRD path should resolve against WorkDir, got: %v", err)
	}
}

func TestPrintDryRun_InvalidRepoPath(t *testing.T) {
	dir := t.TempDir()
	prdPath := filepath.Join(dir, "prd.json")

	prdContent := `{
		"name": "test",
		"tasks": [{"id": "task-1", "title": "Task", "status": "pending"}]
	}`
	if err := os.WriteFile(prdPath, []byte(prdContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := supervisor.DefaultConfig()
	cfg.PRDFile = prdPath
	cfg.WorkDir = "/nonexistent/repo/path"

	err := printDryRun(cfg)
	if err == nil {
		t.Error("printDryRun() with invalid WorkDir should return error")
	}
}

func TestValidateRepo(t *testing.T) {
	validDir := t.TempDir()

	// Create a file (not a directory) for the "path is file" test case.
	filePath := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		repoURL string
		workDir string
		wantErr string // empty means no error expected
	}{
		// Remote URLs — all accepted (validation deferred to workflow layer).
		{"https URL", "https://github.com/user/repo.git", "", ""},
		{"http URL", "http://github.com/user/repo.git", "", ""},
		{"ssh URL", "ssh://git@github.com/user/repo.git", "", ""},
		{"SCP-style SSH URL", "git@github.com:user/repo.git", "", ""},
		{"git protocol URL", "git://github.com/user/repo.git", "", ""},

		// Local paths.
		{"valid local dir", "", validDir, ""},
		{"empty workdir (cwd fallback)", "", "", ""},
		{"nonexistent workdir", "", "/nonexistent/path/xyz", "does not exist"},
		{"workdir is a file", "", filePath, "is not a directory"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &supervisor.Config{
				RepoURL: tt.repoURL,
				WorkDir: tt.workDir,
			}
			err := validateRepo(cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("validateRepo() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("validateRepo() expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("validateRepo() error = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}
