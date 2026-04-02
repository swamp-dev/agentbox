package supervisor

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/swamp-dev/agentbox/internal/journal"
	"github.com/swamp-dev/agentbox/internal/retro"
	"github.com/swamp-dev/agentbox/internal/store"
	"github.com/swamp-dev/agentbox/internal/taskdb"
)

// AdaptiveController applies retrospective recommendations.
type AdaptiveController struct {
	store     *store.Store
	sessionID int64
	taskDB    *taskdb.DB
	logger    *slog.Logger

	// Fallback agent switching.
	fallbackAgent     string
	switchRecommended bool
	switched          bool // idempotency guard — only switch once per session
	journal           *journal.Journal
}

// NewAdaptiveController creates a new adaptive controller.
// The taskDB parameter is optional; if nil, task reordering and splitting are no-ops.
func NewAdaptiveController(s *store.Store, sessionID int64, tdb *taskdb.DB, logger *slog.Logger) *AdaptiveController {
	return &AdaptiveController{store: s, sessionID: sessionID, taskDB: tdb, logger: logger}
}

// SetFallbackAgent configures the fallback agent name.
func (ac *AdaptiveController) SetFallbackAgent(name string) {
	ac.fallbackAgent = name
}

// SetJournal configures the journal for writing agent-switch entries.
func (ac *AdaptiveController) SetJournal(j *journal.Journal) {
	ac.journal = j
}

// SwitchRecommended returns whether an agent switch was recommended and the
// target agent name. The recommendation is cleared after reading.
func (ac *AdaptiveController) SwitchRecommended() (bool, string) {
	if ac.switchRecommended {
		ac.switchRecommended = false
		return true, ac.fallbackAgent
	}
	return false, ""
}

// Apply processes recommendations and returns actions taken.
func (ac *AdaptiveController) Apply(recs []retro.Recommendation) []string {
	var actions []string

	for _, rec := range recs {
		switch rec.Action {
		case retro.RecDeferTask:
			if rec.TaskID != "" {
				if err := ac.store.UpdateTaskStatus(rec.TaskID, "deferred"); err == nil {
					actions = append(actions, fmt.Sprintf("Deferred task %s: %s", rec.TaskID, rec.Description))
					ac.logger.Info("deferred task", "task_id", rec.TaskID)
				}
			}

		case retro.RecSkipTask:
			if rec.TaskID != "" {
				if err := ac.store.UpdateTaskStatus(rec.TaskID, "deferred"); err == nil {
					actions = append(actions, fmt.Sprintf("Skipped task %s: %s", rec.TaskID, rec.Description))
					ac.logger.Info("skipped task", "task_id", rec.TaskID)
				}
			}

		case retro.RecUpdateContext:
			// Append failure pattern info to the task's context notes.
			if rec.TaskID != "" {
				task, err := ac.store.GetTask(rec.TaskID)
				if err == nil {
					note := fmt.Sprintf("\n[Retro %s] %s", time.Now().Format("2006-01-02 15:04"), rec.Description)
					newNotes := task.ContextNotes + note
					if err := ac.store.UpdateTaskContextNotes(rec.TaskID, newNotes); err == nil {
						actions = append(actions, fmt.Sprintf("Updated context for task %s", rec.TaskID))
						ac.logger.Info("updated task context", "task_id", rec.TaskID, "note", note)
					}
				}
			}

		case retro.RecSwitchAgent:
			if ac.fallbackAgent != "" && !ac.switched {
				ac.switchRecommended = true
				ac.switched = true
				actions = append(actions, fmt.Sprintf("Recommended agent switch to %s: %s", ac.fallbackAgent, rec.Description))
				ac.logger.Info("agent switch recommended", "agent", ac.fallbackAgent, "reason", rec.Description)

				if ac.journal != nil {
					if err := ac.journal.Add(&store.JournalEntry{
						Kind:       string(journal.KindAgentSwitch),
						Summary:    fmt.Sprintf("Agent switch recommended to %s", ac.fallbackAgent),
						Reflection: fmt.Sprintf("Adaptive controller recommends switching to fallback agent %q. Reason: %s", ac.fallbackAgent, rec.Description),
					}); err != nil {
						ac.logger.Warn("failed to write agent switch journal entry", "error", err)
					}
				}
			} else if ac.switched {
				actions = append(actions, fmt.Sprintf("Agent switch already applied, skipping: %s", rec.Description))
				ac.logger.Info("agent switch already applied, ignoring duplicate recommendation", "reason", rec.Description)
			} else {
				actions = append(actions, fmt.Sprintf("Recommendation: switch agent — %s", rec.Description))
				ac.logger.Warn("agent switch recommended but no fallback configured", "reason", rec.Description)
			}

		case retro.RecRollback:
			actions = append(actions, fmt.Sprintf("Recommendation: rollback — %s", rec.Description))
			ac.logger.Warn("rollback recommended", "reason", rec.Description)

		case retro.RecEscalate:
			actions = append(actions, fmt.Sprintf("Escalation: %s", rec.Description))
			ac.logger.Warn("escalation needed", "reason", rec.Description)

		case retro.RecReorderTasks:
			if ac.taskDB != nil && rec.TaskID != "" {
				// Lower the failing task's priority so other tasks run first.
				// Higher priority number = lower priority in NextTask() sorting.
				task, ok := ac.taskDB.Get(rec.TaskID)
				if ok {
					newPriority := task.Priority + 10
					if err := ac.taskDB.UpdatePriority(rec.TaskID, newPriority); err == nil {
						actions = append(actions, fmt.Sprintf("Deprioritized task %s (priority %d → %d): %s", rec.TaskID, task.Priority, newPriority, rec.Description))
						ac.logger.Info("deprioritized task", "task_id", rec.TaskID, "old_priority", task.Priority, "new_priority", newPriority)
					} else {
						actions = append(actions, fmt.Sprintf("Recommendation: reorder tasks — %s", rec.Description))
					}
				} else {
					actions = append(actions, fmt.Sprintf("Recommendation: reorder tasks — %s", rec.Description))
				}
			} else {
				actions = append(actions, fmt.Sprintf("Recommendation: reorder tasks — %s", rec.Description))
			}

		case retro.RecSplitTask:
			if ac.taskDB != nil && rec.TaskID != "" {
				task, ok := ac.taskDB.Get(rec.TaskID)
				if ok {
					subtasks := ac.generateSubtasks(task, rec.Description)
					if err := ac.taskDB.SplitTask(rec.TaskID, subtasks); err == nil {
						subtaskIDs := make([]string, len(subtasks))
						for i, st := range subtasks {
							subtaskIDs[i] = st.ID
						}
						actions = append(actions, fmt.Sprintf("Split task %s into %d subtasks (%s): %s",
							rec.TaskID, len(subtasks), strings.Join(subtaskIDs, ", "), rec.Description))
						ac.logger.Info("split task", "task_id", rec.TaskID, "subtasks", len(subtasks))
					} else {
						actions = append(actions, fmt.Sprintf("Recommendation: split task — %s (error: %v)", rec.Description, err))
						ac.logger.Warn("failed to split task", "task_id", rec.TaskID, "error", err)
					}
				} else {
					actions = append(actions, fmt.Sprintf("Recommendation: split task — %s", rec.Description))
				}
			} else {
				actions = append(actions, fmt.Sprintf("Recommendation: split task — %s", rec.Description))
			}
		}
	}

	return actions
}

// generateSubtasks creates subtasks from a parent task for splitting.
// It divides the work into smaller pieces based on the task's description.
func (ac *AdaptiveController) generateSubtasks(parent *taskdb.Task, description string) []*taskdb.Task {
	// Generate 2 subtasks from the parent task.
	return []*taskdb.Task{
		{
			ID:          parent.ID + "-part1",
			Title:       parent.Title + " (part 1)",
			Description: fmt.Sprintf("First part of: %s. Context: %s", parent.Description, description),
			Status:      taskdb.StatusPending,
			Priority:    parent.Priority,
			MaxAttempts: parent.MaxAttempts,
		},
		{
			ID:          parent.ID + "-part2",
			Title:       parent.Title + " (part 2)",
			Description: fmt.Sprintf("Second part of: %s. Context: %s", parent.Description, description),
			Status:      taskdb.StatusPending,
			Priority:    parent.Priority,
			MaxAttempts: parent.MaxAttempts,
		},
	}
}

// WriteEscalation appends an escalation message to the escalation log.
func (ac *AdaptiveController) WriteEscalation(workDir, message string) error {
	path := filepath.Join(workDir, ".agentbox", "escalations.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	entry := fmt.Sprintf("\n## %s\n\n%s\n", time.Now().Format("2006-01-02 15:04:05"), message)
	_, err = f.WriteString(entry)
	return err
}
