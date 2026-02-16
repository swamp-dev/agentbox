// Package taskdb provides rich task management with DAG-based dependency tracking.
package taskdb

import "time"

// TaskStatus represents the lifecycle state of a task.
type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusInProgress TaskStatus = "in_progress"
	StatusCompleted  TaskStatus = "completed"
	StatusFailed     TaskStatus = "failed"
	StatusDeferred   TaskStatus = "deferred"
	StatusBlocked    TaskStatus = "blocked"
)

// Task represents a unit of work with dependencies and execution history.
type Task struct {
	ID                 string              `json:"id"`
	Title              string              `json:"title"`
	Description        string              `json:"description"`
	Status             TaskStatus          `json:"status"`
	Priority           int                 `json:"priority"`
	Complexity         int                 `json:"complexity"`
	ParentID           string              `json:"parent_id,omitempty"`
	DependsOn          []string            `json:"depends_on,omitempty"`
	MaxAttempts        int                 `json:"max_attempts"`
	ContextNotes       string              `json:"context_notes,omitempty"`
	AcceptanceCriteria []AcceptanceCriteria `json:"acceptance_criteria,omitempty"`
	Tags               []string            `json:"tags,omitempty"`
	Attempts           []Attempt           `json:"attempts,omitempty"`
	CreatedAt          time.Time           `json:"created_at"`
	CompletedAt        *time.Time          `json:"completed_at,omitempty"`
}

// AcceptanceCriteria defines a condition for task completion.
type AcceptanceCriteria struct {
	Description string `json:"description"`
	Command     string `json:"command,omitempty"` // Shell command that returns 0 when met.
}

// Attempt records a single execution attempt on a task.
type Attempt struct {
	Number      int        `json:"number"`
	AgentName   string     `json:"agent_name"`
	Success     bool       `json:"success"`
	ErrorMsg    string     `json:"error_msg,omitempty"`
	GitCommit   string     `json:"git_commit,omitempty"`
	GitRollback string     `json:"git_rollback,omitempty"`
	TokensUsed  int        `json:"tokens_used,omitempty"`
	DurationMs  int        `json:"duration_ms,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// HasExhaustedAttempts returns true if the task has used all allowed attempts.
func (t *Task) HasExhaustedAttempts() bool {
	return len(t.Attempts) >= t.MaxAttempts
}

// LastAttempt returns the most recent attempt, or nil if none.
func (t *Task) LastAttempt() *Attempt {
	if len(t.Attempts) == 0 {
		return nil
	}
	return &t.Attempts[len(t.Attempts)-1]
}

// FailureHistory returns error messages from all failed attempts.
func (t *Task) FailureHistory() []string {
	var failures []string
	for _, a := range t.Attempts {
		if !a.Success && a.ErrorMsg != "" {
			failures = append(failures, a.ErrorMsg)
		}
	}
	return failures
}
