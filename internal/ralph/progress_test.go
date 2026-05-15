package ralph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewProgress(t *testing.T) {
	p := NewProgress("/some/path/progress.txt")
	if p == nil {
		t.Fatal("NewProgress returned nil")
	}
	if p.path != "/some/path/progress.txt" {
		t.Errorf("expected path /some/path/progress.txt, got %s", p.path)
	}
	if len(p.entries) != 0 {
		t.Errorf("expected empty entries, got %d", len(p.entries))
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	dir := t.TempDir()
	p := NewProgress(filepath.Join(dir, "nonexistent.txt"))

	err := p.Load()
	if err != nil {
		t.Errorf("expected nil error for non-existent file, got %v", err)
	}
	if len(p.entries) != 0 {
		t.Errorf("expected empty entries, got %d", len(p.entries))
	}
}

func TestLoadAndParseEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.txt")

	content := `# Test Project - Progress Log

## 2024-01-15 10:30:00 - Setup Project
Task ID: task-1
Status: COMPLETED

Task completed successfully

Learnings:
- Go modules are great
- Testing is important

## 2024-01-15 11:00:00 - Add Feature
Task ID: task-2
Status: STARTED
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p := NewProgress(path)
	if err := p.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	entries := p.GetEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// First entry
	e := entries[0]
	if e.TaskID != "task-1" {
		t.Errorf("expected task ID task-1, got %s", e.TaskID)
	}
	if e.TaskTitle != "Setup Project" {
		t.Errorf("expected title Setup Project, got %s", e.TaskTitle)
	}
	if e.Status != "COMPLETED" {
		t.Errorf("expected status COMPLETED, got %s", e.Status)
	}
	if !strings.Contains(e.Message, "Task completed successfully") {
		t.Errorf("expected message to contain 'Task completed successfully', got %s", e.Message)
	}
	if len(e.Learnings) != 2 {
		t.Fatalf("expected 2 learnings, got %d", len(e.Learnings))
	}
	if e.Learnings[0] != "Go modules are great" {
		t.Errorf("expected first learning 'Go modules are great', got %s", e.Learnings[0])
	}

	expectedTime, _ := time.Parse("2006-01-02 15:04:05", "2024-01-15 10:30:00")
	if !e.Timestamp.Equal(expectedTime) {
		t.Errorf("expected timestamp %v, got %v", expectedTime, e.Timestamp)
	}

	// Second entry
	e2 := entries[1]
	if e2.TaskID != "task-2" {
		t.Errorf("expected task ID task-2, got %s", e2.TaskID)
	}
	if e2.Status != "STARTED" {
		t.Errorf("expected status STARTED, got %s", e2.Status)
	}
}

func TestAppend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.txt")

	p := NewProgress(path)

	entry := ProgressEntry{
		Timestamp: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
		TaskID:    "task-1",
		TaskTitle: "Test Task",
		Status:    "COMPLETED",
		Message:   "All done",
		Learnings: []string{"learned stuff"},
	}

	if err := p.Append(entry); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "## 2024-01-15 12:00:00 - Test Task") {
		t.Error("expected timestamp and title in output")
	}
	if !strings.Contains(content, "Task ID: task-1") {
		t.Error("expected task ID in output")
	}
	if !strings.Contains(content, "Status: COMPLETED") {
		t.Error("expected status in output")
	}
	if !strings.Contains(content, "All done") {
		t.Error("expected message in output")
	}
	if !strings.Contains(content, "- learned stuff") {
		t.Error("expected learnings in output")
	}
}

func TestRecordStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.txt")

	p := NewProgress(path)
	if err := p.RecordStart("task-1", "Start Task"); err != nil {
		t.Fatalf("RecordStart() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !strings.Contains(content, "Status: STARTED") {
		t.Error("expected STARTED status")
	}
	if !strings.Contains(content, "Task ID: task-1") {
		t.Error("expected task ID")
	}
	if !strings.Contains(content, "Start Task") {
		t.Error("expected task title")
	}
}

func TestRecordComplete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.txt")

	p := NewProgress(path)
	learnings := []string{"thing one", "thing two"}
	if err := p.RecordComplete("task-2", "Complete Task", "finished", learnings); err != nil {
		t.Fatalf("RecordComplete() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !strings.Contains(content, "Status: COMPLETED") {
		t.Error("expected COMPLETED status")
	}
	if !strings.Contains(content, "finished") {
		t.Error("expected message")
	}
	if !strings.Contains(content, "- thing one") {
		t.Error("expected first learning")
	}
	if !strings.Contains(content, "- thing two") {
		t.Error("expected second learning")
	}
}

func TestRecordFailed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.txt")

	p := NewProgress(path)
	if err := p.RecordFailed("task-3", "Failed Task", "timeout"); err != nil {
		t.Fatalf("RecordFailed() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !strings.Contains(content, "Status: FAILED") {
		t.Error("expected FAILED status")
	}
	if !strings.Contains(content, "timeout") {
		t.Error("expected failure message")
	}
}

func TestRecordIteration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.txt")

	p := NewProgress(path)
	if err := p.RecordIteration(3, "Completed 5 tasks"); err != nil {
		t.Fatalf("RecordIteration() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !strings.Contains(content, "Task ID: iteration-3") {
		t.Error("expected iteration task ID")
	}
	if !strings.Contains(content, "Iteration 3 Summary") {
		t.Error("expected iteration title")
	}
	if !strings.Contains(content, "Status: ITERATION") {
		t.Error("expected ITERATION status")
	}
	if !strings.Contains(content, "Completed 5 tasks") {
		t.Error("expected iteration summary")
	}
}

func TestGetEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.txt")

	p := NewProgress(path)

	// Empty initially
	if len(p.GetEntries()) != 0 {
		t.Error("expected empty entries initially")
	}

	_ = p.RecordStart("task-1", "First")
	_ = p.RecordComplete("task-2", "Second", "done", nil)

	entries := p.GetEntries()
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestSummary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.txt")

	p := NewProgress(path)
	_ = p.RecordStart("task-1", "First")
	_ = p.RecordComplete("task-2", "Second", "done", nil)
	_ = p.RecordFailed("task-3", "Third", "error")

	summary := p.Summary()
	if summary != "Tasks: 1 started, 1 completed, 1 failed" {
		t.Errorf("unexpected summary: %s", summary)
	}
}

func TestSummaryEmpty(t *testing.T) {
	p := NewProgress("/nonexistent")
	summary := p.Summary()
	if summary != "Tasks: 0 started, 0 completed, 0 failed" {
		t.Errorf("unexpected summary: %s", summary)
	}
}

func TestCreateProgressFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.txt")

	if err := CreateProgressFile(path, "My Project"); err != nil {
		t.Fatalf("CreateProgressFile() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !strings.Contains(content, "# My Project - Progress Log") {
		t.Error("expected project header")
	}
	if !strings.Contains(content, "Created:") {
		t.Error("expected created timestamp")
	}
	if !strings.Contains(content, "tracks the progress") {
		t.Error("expected description text")
	}
}

func TestLoadAfterAppend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.txt")

	// Write entries
	p1 := NewProgress(path)
	_ = p1.RecordStart("task-1", "First Task")
	_ = p1.RecordComplete("task-2", "Second Task", "done", []string{"a learning"})

	// Load in a new instance
	p2 := NewProgress(path)
	if err := p2.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	entries := p2.GetEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after reload, got %d", len(entries))
	}

	if entries[0].Status != "STARTED" {
		t.Errorf("expected STARTED, got %s", entries[0].Status)
	}
	if entries[1].Status != "COMPLETED" {
		t.Errorf("expected COMPLETED, got %s", entries[1].Status)
	}
	if len(entries[1].Learnings) != 1 {
		t.Errorf("expected 1 learning, got %d", len(entries[1].Learnings))
	}
}
