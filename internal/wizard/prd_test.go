package wizard

import (
	"strings"
	"testing"
)

func TestGeneratePRDTemplate_CreatesValidPRD(t *testing.T) {
	prd := GeneratePRDTemplate("my-project", "Build a REST API for managing tasks")

	if prd.Name != "my-project" {
		t.Errorf("PRD.Name = %q, want %q", prd.Name, "my-project")
	}

	if prd.Description == "" {
		t.Error("PRD.Description should not be empty")
	}
}

func TestGeneratePRDTemplate_HasCorrectNumberOfTasks(t *testing.T) {
	prd := GeneratePRDTemplate("test-project", "Build a CLI tool")

	if len(prd.Tasks) != 5 {
		t.Errorf("expected 5 tasks, got %d", len(prd.Tasks))
	}
}

func TestGeneratePRDTemplate_TasksIncludeDescription(t *testing.T) {
	description := "Build a REST API for managing tasks"
	prd := GeneratePRDTemplate("my-project", description)

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

func TestGeneratePRDTemplate_AllTasksPending(t *testing.T) {
	prd := GeneratePRDTemplate("test", "Build something")

	for _, task := range prd.Tasks {
		if task.Status != "pending" {
			t.Errorf("task %q status = %q, want %q", task.ID, task.Status, "pending")
		}
	}
}

func TestGeneratePRDTemplate_TasksHaveIDs(t *testing.T) {
	prd := GeneratePRDTemplate("test", "Build something")

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

func TestGeneratePRDTemplate_TasksHaveDependencies(t *testing.T) {
	prd := GeneratePRDTemplate("test", "Build something")

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
