package cli

import (
	"os"
	"path/filepath"
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
