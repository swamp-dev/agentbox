// Package ralph implements the Ralph pattern for iterative AI agent execution.
package ralph

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// PRD represents a Product Requirements Document with tasks.
type PRD struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Tasks       []Task     `json:"tasks"`
	Metadata    PRDMeta    `json:"metadata,omitempty"`
}

// PRDMeta holds metadata about the PRD execution.
type PRDMeta struct {
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	TotalTasks  int       `json:"total_tasks"`
	Completed   int       `json:"completed"`
	InProgress  int       `json:"in_progress"`
	Pending     int       `json:"pending"`
}

// Task represents a single task in the PRD.
type Task struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"` // pending, in_progress, completed, blocked
	Priority    int       `json:"priority,omitempty"`
	DependsOn   []string  `json:"depends_on,omitempty"`
	Subtasks    []Task    `json:"subtasks,omitempty"`
	Learnings   string    `json:"learnings,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// LoadPRD reads and parses a PRD JSON file.
func LoadPRD(path string) (*PRD, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading PRD file: %w", err)
	}

	var prd PRD
	if err := json.Unmarshal(data, &prd); err != nil {
		return nil, fmt.Errorf("parsing PRD file: %w", err)
	}

	prd.updateMetadata()
	return &prd, nil
}

// Save writes the PRD to a JSON file.
func (p *PRD) Save(path string) error {
	p.Metadata.UpdatedAt = time.Now()
	p.updateMetadata()

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling PRD: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing PRD file: %w", err)
	}

	return nil
}

// updateMetadata recalculates task counts.
func (p *PRD) updateMetadata() {
	var total, completed, inProgress, pending int

	var countTasks func(tasks []Task)
	countTasks = func(tasks []Task) {
		for _, t := range tasks {
			total++
			switch t.Status {
			case "completed":
				completed++
			case "in_progress":
				inProgress++
			default:
				pending++
			}
			countTasks(t.Subtasks)
		}
	}

	countTasks(p.Tasks)

	p.Metadata.TotalTasks = total
	p.Metadata.Completed = completed
	p.Metadata.InProgress = inProgress
	p.Metadata.Pending = pending
}

// NextTask returns the next incomplete task that has no blockers.
func (p *PRD) NextTask() *Task {
	completedIDs := make(map[string]bool)

	var collectCompleted func(tasks []Task)
	collectCompleted = func(tasks []Task) {
		for _, t := range tasks {
			if t.Status == "completed" {
				completedIDs[t.ID] = true
			}
			collectCompleted(t.Subtasks)
		}
	}
	collectCompleted(p.Tasks)

	var findNext func(tasks []Task) *Task
	findNext = func(tasks []Task) *Task {
		for i := range tasks {
			t := &tasks[i]

			if t.Status != "pending" && t.Status != "in_progress" {
				continue
			}

			blocked := false
			for _, dep := range t.DependsOn {
				if !completedIDs[dep] {
					blocked = true
					break
				}
			}
			if blocked {
				continue
			}

			if len(t.Subtasks) > 0 {
				if sub := findNext(t.Subtasks); sub != nil {
					return sub
				}
			}

			return t
		}
		return nil
	}

	return findNext(p.Tasks)
}

// MarkTaskComplete marks a task as completed and records learnings.
func (p *PRD) MarkTaskComplete(taskID string, learnings string) error {
	var markComplete func(tasks []Task) bool
	markComplete = func(tasks []Task) bool {
		for i := range tasks {
			if tasks[i].ID == taskID {
				tasks[i].Status = "completed"
				tasks[i].CompletedAt = time.Now()
				tasks[i].Learnings = learnings
				return true
			}
			if markComplete(tasks[i].Subtasks) {
				return true
			}
		}
		return false
	}

	if !markComplete(p.Tasks) {
		return fmt.Errorf("task not found: %s", taskID)
	}

	p.updateMetadata()
	return nil
}

// MarkTaskInProgress marks a task as in progress.
func (p *PRD) MarkTaskInProgress(taskID string) error {
	var mark func(tasks []Task) bool
	mark = func(tasks []Task) bool {
		for i := range tasks {
			if tasks[i].ID == taskID {
				tasks[i].Status = "in_progress"
				return true
			}
			if mark(tasks[i].Subtasks) {
				return true
			}
		}
		return false
	}

	if !mark(p.Tasks) {
		return fmt.Errorf("task not found: %s", taskID)
	}

	p.updateMetadata()
	return nil
}

// IsComplete returns true if all tasks are completed.
func (p *PRD) IsComplete() bool {
	return p.Metadata.Pending == 0 && p.Metadata.InProgress == 0
}

// Progress returns the completion percentage.
func (p *PRD) Progress() float64 {
	if p.Metadata.TotalTasks == 0 {
		return 100.0
	}
	return float64(p.Metadata.Completed) / float64(p.Metadata.TotalTasks) * 100.0
}

// GetTask returns a task by ID.
func (p *PRD) GetTask(taskID string) *Task {
	var find func(tasks []Task) *Task
	find = func(tasks []Task) *Task {
		for i := range tasks {
			if tasks[i].ID == taskID {
				return &tasks[i]
			}
			if t := find(tasks[i].Subtasks); t != nil {
				return t
			}
		}
		return nil
	}

	return find(p.Tasks)
}

// CreateDefaultPRD creates a sample PRD for initialization.
func CreateDefaultPRD(projectName string) *PRD {
	return &PRD{
		Name:        projectName,
		Description: "Project requirements and tasks",
		Tasks: []Task{
			{
				ID:          "task-1",
				Title:       "Initial Setup",
				Description: "Set up the project structure and dependencies",
				Status:      "pending",
				Priority:    1,
			},
			{
				ID:          "task-2",
				Title:       "Core Implementation",
				Description: "Implement the main functionality",
				Status:      "pending",
				Priority:    2,
				DependsOn:   []string{"task-1"},
			},
			{
				ID:          "task-3",
				Title:       "Testing",
				Description: "Add tests for the implementation",
				Status:      "pending",
				Priority:    3,
				DependsOn:   []string{"task-2"},
			},
		},
		Metadata: PRDMeta{
			CreatedAt: time.Now(),
		},
	}
}
