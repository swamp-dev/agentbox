package supervisor

import (
	"fmt"
	"strings"

	"github.com/swamp-dev/agentbox/internal/store"
	"github.com/swamp-dev/agentbox/internal/taskdb"
)

// ContextBuilder constructs enriched prompts for the coding agent.
type ContextBuilder struct {
	store     *store.Store
	sessionID int64
}

// NewContextBuilder creates a new context builder.
func NewContextBuilder(s *store.Store, sessionID int64) *ContextBuilder {
	return &ContextBuilder{store: s, sessionID: sessionID}
}

// BuildPrompt constructs a rich prompt including task context, failure history,
// completed work summary, and acceptance criteria.
func (cb *ContextBuilder) BuildPrompt(task *taskdb.Task, projectName string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are working on: %s\n\n", projectName))

	// Current task.
	sb.WriteString("## Current Task\n")
	sb.WriteString(fmt.Sprintf("ID: %s\n", task.ID))
	sb.WriteString(fmt.Sprintf("Title: %s\n", task.Title))
	sb.WriteString(fmt.Sprintf("Description: %s\n", task.Description))
	sb.WriteString("\n")

	// Acceptance criteria.
	if len(task.AcceptanceCriteria) > 0 {
		sb.WriteString("## Acceptance Criteria\n")
		for i, ac := range task.AcceptanceCriteria {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, ac.Description))
			if ac.Command != "" {
				sb.WriteString(fmt.Sprintf("   Verify: `%s`\n", ac.Command))
			}
		}
		sb.WriteString("\n")
	}

	// Context notes.
	if task.ContextNotes != "" {
		sb.WriteString("## Additional Context\n")
		sb.WriteString(task.ContextNotes)
		sb.WriteString("\n\n")
	}

	// Failure history.
	failures := task.FailureHistory()
	if len(failures) > 0 {
		sb.WriteString("## Previous Attempt Failures (DO NOT REPEAT)\n")
		for i, f := range failures {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, f))
		}
		sb.WriteString("\n")
	}

	// Completed work context.
	cb.appendCompletedContext(&sb)

	// Known failing tests.
	cb.appendFailingTests(&sb)

	// Instructions.
	sb.WriteString("## Instructions\n")
	sb.WriteString("1. Complete the task described above\n")
	sb.WriteString("2. Make small, focused changes\n")
	sb.WriteString("3. Ensure your changes are complete and tested\n")
	sb.WriteString("4. When the task is FULLY complete, output: <promise>COMPLETE</promise>\n\n")
	sb.WriteString("Important: Only output the completion signal when the task is truly done.\n")

	return sb.String()
}

// appendCompletedContext adds a summary of completed tasks.
func (cb *ContextBuilder) appendCompletedContext(sb *strings.Builder) {
	tasks, err := cb.store.ListTasks(cb.sessionID)
	if err != nil {
		return
	}

	var completed []string
	for _, t := range tasks {
		if t.Status == "completed" {
			completed = append(completed, fmt.Sprintf("- %s: %s", t.ID, t.Title))
		}
	}

	if len(completed) > 0 {
		sb.WriteString("## Already Completed\n")
		for _, c := range completed {
			sb.WriteString(c + "\n")
		}
		sb.WriteString("\n")
	}
}

// appendFailingTests adds known failing test information.
func (cb *ContextBuilder) appendFailingTests(sb *strings.Builder) {
	failing, err := cb.store.FailingTestTrend(cb.sessionID, 5)
	if err != nil || len(failing) == 0 {
		return
	}

	sb.WriteString("## Known Failing Tests\n")
	for test, count := range failing {
		sb.WriteString(fmt.Sprintf("- %s (failed %d times recently)\n", test, count))
	}
	sb.WriteString("\n")
}
