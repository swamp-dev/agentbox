package wizard

import (
	"strings"
	"testing"
)

func TestGeneratePRD_CreatesValidPRD(t *testing.T) {
	prd := GeneratePRD("my-project", "Build a REST API for managing tasks")

	if prd.Name != "my-project" {
		t.Errorf("PRD.Name = %q, want %q", prd.Name, "my-project")
	}

	if prd.Description == "" {
		t.Error("PRD.Description should not be empty")
	}
}

func TestGeneratePRD_HasCorrectNumberOfTasks(t *testing.T) {
	prd := GeneratePRD("test-project", "Build a CLI tool")

	if len(prd.Tasks) < 3 || len(prd.Tasks) > 5 {
		t.Errorf("expected 3-5 tasks, got %d", len(prd.Tasks))
	}
}

func TestGeneratePRD_TasksIncludeDescription(t *testing.T) {
	description := "Build a REST API for managing tasks"
	prd := GeneratePRD("my-project", description)

	foundDescRef := false
	for _, task := range prd.Tasks {
		if strings.Contains(task.Description, description) || strings.Contains(task.Title, "REST API") {
			foundDescRef = true
			break
		}
	}

	if !foundDescRef {
		t.Error("at least one task should reference the user's description")
	}
}

func TestGeneratePRD_AllTasksPending(t *testing.T) {
	prd := GeneratePRD("test", "Build something")

	for _, task := range prd.Tasks {
		if task.Status != "pending" {
			t.Errorf("task %q status = %q, want %q", task.ID, task.Status, "pending")
		}
	}
}

func TestGeneratePRD_TasksHaveIDs(t *testing.T) {
	prd := GeneratePRD("test", "Build something")

	ids := map[string]bool{}
	for _, task := range prd.Tasks {
		if task.ID == "" {
			t.Error("task ID should not be empty")
		}
		if ids[task.ID] {
			t.Errorf("duplicate task ID: %s", task.ID)
		}
		ids[task.ID] = true
	}
}

func TestGeneratePRD_TasksHaveDependencies(t *testing.T) {
	prd := GeneratePRD("test", "Build something")

	// First task should have no dependencies
	if len(prd.Tasks[0].DependsOn) != 0 {
		t.Error("first task should have no dependencies")
	}

	// Later tasks should depend on earlier ones
	hasDeps := false
	for _, task := range prd.Tasks[1:] {
		if len(task.DependsOn) > 0 {
			hasDeps = true
			break
		}
	}
	if !hasDeps {
		t.Error("at least one later task should have dependencies")
	}
}
