package journal

import (
	"strings"
	"testing"

	"github.com/swamp-dev/agentbox/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("opening test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestJournalAddAndEntries(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	j := New(s, sessionID)

	err := j.Add(&store.JournalEntry{
		Kind:       string(KindTaskComplete),
		TaskID:     "task-1",
		Sprint:     1,
		Iteration:  1,
		Summary:    "Completed auth setup",
		Reflection: "This went smoothly.",
		Confidence: 4,
		Difficulty: 2,
		Momentum:   4,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	entries, err := j.Entries(nil)
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Summary != "Completed auth setup" {
		t.Errorf("unexpected summary: %q", entries[0].Summary)
	}
}

func TestJournalExportMarkdown(t *testing.T) {
	s := openTestStore(t)
	sessionID, _ := s.CreateSession("", "main", "")

	j := New(s, sessionID)
	if err := j.Add(&store.JournalEntry{
		Kind:       string(KindTaskComplete),
		Sprint:     1,
		Iteration:  1,
		Summary:    "Setup complete",
		Reflection: "Everything worked on the first try.",
		Confidence: 5,
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	md, err := j.ExportMarkdown()
	if err != nil {
		t.Fatalf("ExportMarkdown: %v", err)
	}
	if !strings.Contains(md, "Setup complete") {
		t.Error("expected markdown to contain entry summary")
	}
	if !strings.Contains(md, "Confidence: 5/5") {
		t.Error("expected markdown to contain confidence")
	}
}

func TestBuildReflectionPrompt(t *testing.T) {
	prompt := BuildReflectionPrompt("Add auth middleware", true, 1, "", "improving", "Add role checks")
	if !strings.Contains(prompt, "Add auth middleware") {
		t.Error("expected task title in prompt")
	}
	if !strings.Contains(prompt, "Completed successfully") {
		t.Error("expected success indicator")
	}
	if !strings.Contains(prompt, "Add role checks") {
		t.Error("expected next task in prompt")
	}
}

func TestBuildReflectionPrompt_Failure(t *testing.T) {
	prompt := BuildReflectionPrompt("Fix bug", false, 2, "test failed", "degrading", "")
	if !strings.Contains(prompt, "Failed on attempt 2") {
		t.Error("expected failure indicator")
	}
	if !strings.Contains(prompt, "test failed") {
		t.Error("expected error message")
	}
}

func TestRenderEntry(t *testing.T) {
	entry := &store.JournalEntry{
		Iteration:  3,
		Summary:    "Auth middleware done",
		Sprint:     1,
		Reflection: "Went well.",
		Confidence: 4,
		Difficulty: 3,
		Momentum:   4,
	}
	rendered := RenderEntry(entry)
	if !strings.Contains(rendered, "Iteration 3") {
		t.Error("expected iteration number")
	}
	if !strings.Contains(rendered, "Difficulty: 3/5") {
		t.Error("expected difficulty")
	}
}
