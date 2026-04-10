package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
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

	output := captureStdout(t, func() {
		if err := printStatusJSON(prd, progress, false); err != nil {
			t.Errorf("printStatusJSON() error: %v", err)
		}
	})

	if !strings.Contains(output, `"project"`) {
		t.Error("JSON output missing project field")
	}
	if strings.Contains(output, `"task_list"`) {
		t.Error("JSON output should not contain task_list when includeTasks=false")
	}
}

func TestPrintStatusJSON_WithTasks(t *testing.T) {
	prd := loadTestPRD(t)
	progress := ralph.NewProgress(filepath.Join(t.TempDir(), "progress.txt"))

	output := captureStdout(t, func() {
		if err := printStatusJSON(prd, progress, true); err != nil {
			t.Errorf("printStatusJSON(tasks=true) error: %v", err)
		}
	})

	if !strings.Contains(output, `"task_list"`) {
		t.Error("JSON output should contain task_list when includeTasks=true")
	}
	if !strings.Contains(output, `"task-1"`) {
		t.Error("JSON output should contain task IDs")
	}
}

// captureStdout captures stdout output from fn.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		w.Close()
		os.Stdout = old
	}()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// writeTempPRD writes PRD JSON to a temp file and loads it.
func writeTempPRD(t *testing.T, content string) *ralph.PRD {
	t.Helper()
	dir := t.TempDir()
	prdPath := filepath.Join(dir, "prd.json")
	if err := os.WriteFile(prdPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	prd, err := ralph.LoadPRD(prdPath)
	if err != nil {
		t.Fatal(err)
	}
	return prd
}

func TestPrintTaskList(t *testing.T) {
	tests := []struct {
		name       string
		prdJSON    string
		wantIcons  []string // status icons expected in output
		wantIDs    []string // task IDs expected in output
		wantAbsent []string // strings that must NOT appear
	}{
		{
			name: "mixed statuses",
			prdJSON: `{
				"name": "mixed-project",
				"tasks": [
					{"id": "task-1", "title": "Setup environment", "status": "done"},
					{"id": "task-2", "title": "Implement feature", "status": "in_progress"},
					{"id": "task-3", "title": "Write tests", "status": "pending"}
				]
			}`,
			wantIcons: []string{"✓", "▶", "○"},
			wantIDs:   []string{"task-1", "task-2", "task-3"},
		},
		{
			name:    "empty task list",
			prdJSON: `{"name": "empty-project", "tasks": []}`,
			// Should print header but no task lines
			wantIcons:  nil,
			wantIDs:    nil,
			wantAbsent: []string{"task-"},
		},
		{
			name: "all tasks completed",
			prdJSON: `{
				"name": "done-project",
				"tasks": [
					{"id": "task-1", "title": "First task", "status": "done"},
					{"id": "task-2", "title": "Second task", "status": "done"},
					{"id": "task-3", "title": "Third task", "status": "done"}
				]
			}`,
			wantIcons:  []string{"✓"},
			wantIDs:    []string{"task-1", "task-2", "task-3"},
			wantAbsent: []string{"▶", "○"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prd := writeTempPRD(t, tt.prdJSON)

			output := captureStdout(t, func() {
				printTaskList(prd)
			})

			// Must always contain the header
			if !strings.Contains(output, "Tasks:") {
				t.Error("output missing 'Tasks:' header")
			}

			for _, icon := range tt.wantIcons {
				if !strings.Contains(output, icon) {
					t.Errorf("output missing expected icon %q", icon)
				}
			}

			for _, id := range tt.wantIDs {
				if !strings.Contains(output, id) {
					t.Errorf("output missing expected task ID %q", id)
				}
			}

			for _, absent := range tt.wantAbsent {
				if strings.Contains(output, absent) {
					t.Errorf("output should not contain %q", absent)
				}
			}
		})
	}
}

func TestStatusIcon_TaskStatuses(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   string
	}{
		{"done", "done", "✓"},
		{"in_progress", "in_progress", "▶"},
		{"pending", "pending", "○"},
		{"blocked", "blocked", "✗"},
		{"COMPLETED", "COMPLETED", "✓"},
		{"STARTED", "STARTED", "▶"},
		{"FAILED", "FAILED", "✗"},
		{"ITERATION", "ITERATION", "↻"},
		{"unknown", "something", "○"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusIcon(tt.status)
			if got != tt.want {
				t.Errorf("statusIcon(%s) = %s, want %s", tt.status, got, tt.want)
			}
		})
	}
}
