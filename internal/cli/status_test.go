package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/swamp-dev/agentbox/internal/ralph"
)

func loadTestPRD(t *testing.T) *ralph.PRD {
	t.Helper()
	dir := t.TempDir()
	prdPath := filepath.Join(dir, "prd.json")

	prdContent := `{
		"name": "test-project",
		"description": "A test PRD",
		"tasks": [
			{"id": "task-1", "title": "Setup environment", "status": "done", "priority": 1},
			{"id": "task-2", "title": "Implement feature", "status": "in_progress", "priority": 2, "depends_on": ["task-1"]},
			{"id": "task-3", "title": "Write tests", "status": "pending", "priority": 3, "depends_on": ["task-2"]}
		]
	}`
	if err := os.WriteFile(prdPath, []byte(prdContent), 0644); err != nil {
		t.Fatal(err)
	}

	prd, err := ralph.LoadPRD(prdPath)
	if err != nil {
		t.Fatal(err)
	}
	return prd
}

func TestPrintStatusText(t *testing.T) {
	prd := loadTestPRD(t)
	progress := ralph.NewProgress(filepath.Join(t.TempDir(), "progress.txt"))

	if err := printStatusText(prd, progress); err != nil {
		t.Errorf("printStatusText() error: %v", err)
	}
}

func TestPrintStatusText_AllComplete(t *testing.T) {
	dir := t.TempDir()
	prdPath := filepath.Join(dir, "prd.json")

	prdContent := `{
		"name": "complete-project",
		"tasks": [
			{"id": "task-1", "title": "Done", "status": "done"}
		]
	}`
	if err := os.WriteFile(prdPath, []byte(prdContent), 0644); err != nil {
		t.Fatal(err)
	}

	prd, err := ralph.LoadPRD(prdPath)
	if err != nil {
		t.Fatal(err)
	}

	progress := ralph.NewProgress(filepath.Join(t.TempDir(), "progress.txt"))

	if err := printStatusText(prd, progress); err != nil {
		t.Errorf("printStatusText() error: %v", err)
	}
}

func TestPrintStatusJSON(t *testing.T) {
	prd := loadTestPRD(t)
	progress := ralph.NewProgress(filepath.Join(t.TempDir(), "progress.txt"))

	if err := printStatusJSON(prd, progress); err != nil {
		t.Errorf("printStatusJSON() error: %v", err)
	}
}
