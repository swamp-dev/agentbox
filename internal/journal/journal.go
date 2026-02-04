// Package journal provides dev diary functionality for agentbox sessions.
package journal

import (
	"fmt"
	"strings"

	"github.com/swamp-dev/agentbox/internal/store"
)

// EntryKind categorizes journal entries.
type EntryKind string

const (
	KindTaskStart      EntryKind = "task_start"
	KindTaskComplete   EntryKind = "task_complete"
	KindTaskFailed     EntryKind = "task_failed"
	KindSprintRetro    EntryKind = "sprint_retro"
	KindReviewReceived EntryKind = "review_received"
	KindReflection     EntryKind = "reflection"
	KindFinalWrapUp    EntryKind = "final_wrap_up"
)

// Journal is a thin wrapper over store for managing dev diary entries.
type Journal struct {
	store     *store.Store
	sessionID int64
}

// New creates a new journal for the given session.
func New(s *store.Store, sessionID int64) *Journal {
	return &Journal{store: s, sessionID: sessionID}
}

// Add writes a journal entry to the store.
func (j *Journal) Add(entry *store.JournalEntry) error {
	entry.SessionID = j.sessionID
	return j.store.AddJournalEntry(entry)
}

// Entries returns journal entries, optionally filtered.
func (j *Journal) Entries(opts *store.JournalQuery) ([]*store.JournalEntry, error) {
	return j.store.JournalEntries(j.sessionID, opts)
}

// ExportMarkdown generates a human-readable markdown diary.
func (j *Journal) ExportMarkdown() (string, error) {
	return j.store.ExportJournalMarkdown(j.sessionID)
}

// BuildReflectionPrompt creates a prompt for the agent to write a diary entry.
func BuildReflectionPrompt(taskTitle string, success bool, attemptNum int, errorMsg string, qualityTrend string, nextTaskTitle string) string {
	var sb strings.Builder

	sb.WriteString("Write a brief, honest dev diary entry reflecting on what just happened.\n\n")
	sb.WriteString(fmt.Sprintf("Task: %s\n", taskTitle))

	if success {
		sb.WriteString(fmt.Sprintf("Result: Completed successfully on attempt %d\n", attemptNum))
	} else {
		sb.WriteString(fmt.Sprintf("Result: Failed on attempt %d\n", attemptNum))
		if errorMsg != "" {
			sb.WriteString(fmt.Sprintf("Error: %s\n", errorMsg))
		}
	}

	if qualityTrend != "" {
		sb.WriteString(fmt.Sprintf("Quality trend: %s\n", qualityTrend))
	}

	if nextTaskTitle != "" {
		sb.WriteString(fmt.Sprintf("Next task: %s\n", nextTaskTitle))
	}

	sb.WriteString("\nRespond with JSON:\n")
	sb.WriteString(`{"reflection": "Your freeform thoughts...", "confidence": N, "difficulty": N, "momentum": N}`)
	sb.WriteString("\n\nconfidence/difficulty/momentum are 1-5 integers.\n")
	sb.WriteString("Be honest and specific. Mention what surprised you, what was hard, what you'd do differently.\n")

	return sb.String()
}

// RenderEntry formats a single journal entry as markdown.
func RenderEntry(e *store.JournalEntry) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Iteration %d — %s\n", e.Iteration, e.Summary))
	sb.WriteString(fmt.Sprintf("**%s | Sprint %d", e.Timestamp.Format("2006-01-02 15:04"), e.Sprint))
	if e.Confidence > 0 {
		sb.WriteString(fmt.Sprintf(" | Confidence: %d/5", e.Confidence))
	}
	if e.Difficulty > 0 {
		sb.WriteString(fmt.Sprintf(" | Difficulty: %d/5", e.Difficulty))
	}
	if e.Momentum > 0 {
		sb.WriteString(fmt.Sprintf(" | Momentum: %d/5", e.Momentum))
	}
	sb.WriteString("**\n\n")
	sb.WriteString(e.Reflection)
	sb.WriteString("\n")

	return sb.String()
}
