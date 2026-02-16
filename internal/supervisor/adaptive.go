package supervisor

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/swamp-dev/agentbox/internal/retro"
	"github.com/swamp-dev/agentbox/internal/store"
)

// AdaptiveController applies retrospective recommendations.
type AdaptiveController struct {
	store     *store.Store
	sessionID int64
	logger    *slog.Logger
}

// NewAdaptiveController creates a new adaptive controller.
func NewAdaptiveController(s *store.Store, sessionID int64, logger *slog.Logger) *AdaptiveController {
	return &AdaptiveController{store: s, sessionID: sessionID, logger: logger}
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
			actions = append(actions, fmt.Sprintf("Recommendation: switch agent — %s", rec.Description))
			ac.logger.Warn("agent switch recommended", "reason", rec.Description)

		case retro.RecRollback:
			actions = append(actions, fmt.Sprintf("Recommendation: rollback — %s", rec.Description))
			ac.logger.Warn("rollback recommended", "reason", rec.Description)

		case retro.RecEscalate:
			actions = append(actions, fmt.Sprintf("Escalation: %s", rec.Description))
			ac.logger.Warn("escalation needed", "reason", rec.Description)

		case retro.RecReorderTasks:
			actions = append(actions, fmt.Sprintf("Recommendation: reorder tasks — %s", rec.Description))

		case retro.RecSplitTask:
			actions = append(actions, fmt.Sprintf("Recommendation: split task — %s", rec.Description))
		}
	}

	return actions
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
