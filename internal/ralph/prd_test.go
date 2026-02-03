package ralph

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateDefaultPRD(t *testing.T) {
	prd := CreateDefaultPRD("test-project")

	if prd.Name != "test-project" {
		t.Errorf("expected name test-project, got %s", prd.Name)
	}

	if len(prd.Tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(prd.Tasks))
	}
}

func TestPRDNextTask(t *testing.T) {
	prd := CreateDefaultPRD("test")

	next := prd.NextTask()
	if next == nil {
		t.Fatal("expected a next task")
	}

	if next.ID != "task-1" {
		t.Errorf("expected first task, got %s", next.ID)
	}

	_ = prd.MarkTaskComplete("task-1", "done")
	next = prd.NextTask()
	if next == nil {
		t.Fatal("expected a next task")
	}

	if next.ID != "task-2" {
		t.Errorf("expected second task, got %s", next.ID)
	}
}

func TestPRDBlockedTask(t *testing.T) {
	prd := &PRD{
		Name: "test",
		Tasks: []Task{
			{ID: "task-1", Title: "First", Status: "pending"},
			{ID: "task-2", Title: "Second", Status: "pending", DependsOn: []string{"task-1"}},
		},
	}

	next := prd.NextTask()
	if next.ID != "task-1" {
		t.Errorf("expected task-1, got %s", next.ID)
	}

	_ = prd.MarkTaskComplete("task-1", "")
	next = prd.NextTask()
	if next.ID != "task-2" {
		t.Errorf("expected task-2 after unblocking, got %s", next.ID)
	}
}

func TestPRDIsComplete(t *testing.T) {
	prd := &PRD{
		Name: "test",
		Tasks: []Task{
			{ID: "task-1", Title: "First", Status: "pending"},
		},
	}
	prd.updateMetadata()

	if prd.IsComplete() {
		t.Error("expected not complete")
	}

	_ = prd.MarkTaskComplete("task-1", "")
	if !prd.IsComplete() {
		t.Error("expected complete")
	}
}

func TestPRDProgress(t *testing.T) {
	prd := &PRD{
		Name: "test",
		Tasks: []Task{
			{ID: "task-1", Status: "completed"},
			{ID: "task-2", Status: "pending"},
		},
	}
	prd.updateMetadata()

	progress := prd.Progress()
	if progress != 50.0 {
		t.Errorf("expected 50%% progress, got %.1f%%", progress)
	}
}

func TestPRDSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prd.json")

	prd := CreateDefaultPRD("test-project")
	_ = prd.MarkTaskComplete("task-1", "learned something")

	if err := prd.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := LoadPRD(path)
	if err != nil {
		t.Fatalf("LoadPRD() error = %v", err)
	}

	if loaded.Name != prd.Name {
		t.Errorf("expected name %s, got %s", prd.Name, loaded.Name)
	}

	task := loaded.GetTask("task-1")
	if task == nil {
		t.Fatal("expected to find task-1")
	}

	if task.Status != "completed" {
		t.Errorf("expected status completed, got %s", task.Status)
	}

	if task.Learnings != "learned something" {
		t.Errorf("expected learnings, got %s", task.Learnings)
	}
}

func TestPRDSubtasks(t *testing.T) {
	prd := &PRD{
		Name: "test",
		Tasks: []Task{
			{
				ID:     "task-1",
				Title:  "Parent",
				Status: "pending",
				Subtasks: []Task{
					{ID: "task-1.1", Title: "Child 1", Status: "pending"},
					{ID: "task-1.2", Title: "Child 2", Status: "pending"},
				},
			},
		},
	}
	prd.updateMetadata()

	if prd.Metadata.TotalTasks != 3 {
		t.Errorf("expected 3 total tasks, got %d", prd.Metadata.TotalTasks)
	}

	next := prd.NextTask()
	if next.ID != "task-1.1" {
		t.Errorf("expected first subtask, got %s", next.ID)
	}
}

func TestLoadPRDNotFound(t *testing.T) {
	_, err := LoadPRD("/nonexistent/prd.json")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestLoadPRDInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prd.json")

	if err := os.WriteFile(path, []byte("invalid json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadPRD(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
