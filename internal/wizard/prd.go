package wizard

import (
	"fmt"
	"time"

	"github.com/swamp-dev/agentbox/internal/ralph"
)

// GeneratePRD creates a PRD from a project name and user description,
// decomposing the description into standard development phases.
func GeneratePRD(name, description string) *ralph.PRD {
	tasks := []ralph.Task{
		{
			ID:          "task-1",
			Title:       "Project Setup",
			Description: fmt.Sprintf("Set up project structure and dependencies for: %s", description),
			Status:      "pending",
			Priority:    1,
		},
		{
			ID:          "task-2",
			Title:       "Core Implementation",
			Description: fmt.Sprintf("Implement core functionality: %s", description),
			Status:      "pending",
			Priority:    2,
			DependsOn:   []string{"task-1"},
		},
		{
			ID:          "task-3",
			Title:       "Error Handling and Edge Cases",
			Description: "Add error handling and edge cases",
			Status:      "pending",
			Priority:    3,
			DependsOn:   []string{"task-2"},
		},
		{
			ID:          "task-4",
			Title:       "Testing",
			Description: fmt.Sprintf("Write tests for: %s", description),
			Status:      "pending",
			Priority:    4,
			DependsOn:   []string{"task-2"},
		},
		{
			ID:          "task-5",
			Title:       "Documentation and Cleanup",
			Description: "Documentation and cleanup",
			Status:      "pending",
			Priority:    5,
			DependsOn:   []string{"task-3", "task-4"},
		},
	}

	return &ralph.PRD{
		Name:        name,
		Description: description,
		Tasks:       tasks,
		Metadata: ralph.PRDMeta{
			CreatedAt: time.Now(),
		},
	}
}
