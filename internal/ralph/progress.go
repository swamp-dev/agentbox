package ralph

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

// ProgressEntry represents a single entry in the progress file.
type ProgressEntry struct {
	Timestamp time.Time
	TaskID    string
	TaskTitle string
	Status    string
	Message   string
	Learnings []string
}

// Progress manages the progress.txt file.
type Progress struct {
	path    string
	entries []ProgressEntry
}

// NewProgress creates a new Progress manager.
func NewProgress(path string) *Progress {
	return &Progress{
		path:    path,
		entries: []ProgressEntry{},
	}
}

// Load reads the progress file if it exists.
func (p *Progress) Load() error {
	file, err := os.Open(p.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("opening progress file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var current *ProgressEntry

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "## ") {
			if current != nil {
				p.entries = append(p.entries, *current)
			}
			current = &ProgressEntry{}

			parts := strings.SplitN(line, " - ", 2)
			if len(parts) == 2 {
				current.Timestamp, _ = time.Parse("2006-01-02 15:04:05", strings.TrimPrefix(parts[0], "## "))
				current.TaskTitle = parts[1]
			}
		} else if current != nil {
			if strings.HasPrefix(line, "Status: ") {
				current.Status = strings.TrimPrefix(line, "Status: ")
			} else if strings.HasPrefix(line, "Task ID: ") {
				current.TaskID = strings.TrimPrefix(line, "Task ID: ")
			} else if strings.HasPrefix(line, "- ") {
				current.Learnings = append(current.Learnings, strings.TrimPrefix(line, "- "))
			} else if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "Learnings:") {
				current.Message += line + "\n"
			}
		}
	}

	if current != nil {
		p.entries = append(p.entries, *current)
	}

	return scanner.Err()
}

// Append adds a new entry to the progress file.
func (p *Progress) Append(entry ProgressEntry) error {
	p.entries = append(p.entries, entry)

	file, err := os.OpenFile(p.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening progress file: %w", err)
	}
	defer file.Close()

	timestamp := entry.Timestamp.Format("2006-01-02 15:04:05")
	fmt.Fprintf(file, "\n## %s - %s\n", timestamp, entry.TaskTitle)
	fmt.Fprintf(file, "Task ID: %s\n", entry.TaskID)
	fmt.Fprintf(file, "Status: %s\n", entry.Status)

	if entry.Message != "" {
		fmt.Fprintf(file, "\n%s\n", strings.TrimSpace(entry.Message))
	}

	if len(entry.Learnings) > 0 {
		fmt.Fprintln(file, "\nLearnings:")
		for _, l := range entry.Learnings {
			fmt.Fprintf(file, "- %s\n", l)
		}
	}

	fmt.Fprintln(file, "")

	return nil
}

// RecordStart records the start of a task.
func (p *Progress) RecordStart(taskID, taskTitle string) error {
	return p.Append(ProgressEntry{
		Timestamp: time.Now(),
		TaskID:    taskID,
		TaskTitle: taskTitle,
		Status:    "STARTED",
	})
}

// RecordComplete records the completion of a task.
func (p *Progress) RecordComplete(taskID, taskTitle, message string, learnings []string) error {
	return p.Append(ProgressEntry{
		Timestamp: time.Now(),
		TaskID:    taskID,
		TaskTitle: taskTitle,
		Status:    "COMPLETED",
		Message:   message,
		Learnings: learnings,
	})
}

// RecordFailed records a failed task attempt.
func (p *Progress) RecordFailed(taskID, taskTitle, message string) error {
	return p.Append(ProgressEntry{
		Timestamp: time.Now(),
		TaskID:    taskID,
		TaskTitle: taskTitle,
		Status:    "FAILED",
		Message:   message,
	})
}

// RecordIteration records an iteration summary.
func (p *Progress) RecordIteration(iteration int, summary string) error {
	return p.Append(ProgressEntry{
		Timestamp: time.Now(),
		TaskID:    fmt.Sprintf("iteration-%d", iteration),
		TaskTitle: fmt.Sprintf("Iteration %d Summary", iteration),
		Status:    "ITERATION",
		Message:   summary,
	})
}

// GetEntries returns all progress entries.
func (p *Progress) GetEntries() []ProgressEntry {
	return p.entries
}

// Summary returns a summary of the progress.
func (p *Progress) Summary() string {
	var started, completed, failed int

	for _, e := range p.entries {
		switch e.Status {
		case "STARTED":
			started++
		case "COMPLETED":
			completed++
		case "FAILED":
			failed++
		}
	}

	return fmt.Sprintf("Tasks: %d started, %d completed, %d failed", started, completed, failed)
}

// CreateProgressFile creates a new progress file with a header.
func CreateProgressFile(path, projectName string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating progress file: %w", err)
	}
	defer file.Close()

	fmt.Fprintf(file, "# %s - Progress Log\n", projectName)
	fmt.Fprintf(file, "Created: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintln(file, "\nThis file tracks the progress of the Ralph loop execution.")
	fmt.Fprintln(file, "Each entry represents a task that was started, completed, or failed.")
	fmt.Fprintln(file, "")

	return nil
}
