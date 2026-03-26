package wizard

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWizardRun_BasicFlow(t *testing.T) {
	dir := t.TempDir()

	// Simulate user typing: description, then accepting defaults
	input := "Build a REST API for todo items\n\n\n"
	stdin := strings.NewReader(input)
	stdout := &bytes.Buffer{}

	w := &Wizard{
		Dir:    dir,
		Stdin:  stdin,
		Stdout: stdout,
	}

	result, err := w.Run()
	if err != nil {
		t.Fatalf("Wizard.Run() error: %v", err)
	}

	if result.Description != "Build a REST API for todo items" {
		t.Errorf("Description = %q, want %q", result.Description, "Build a REST API for todo items")
	}

	// Language should be detected (full for empty dir)
	if result.Language == "" {
		t.Error("Language should not be empty")
	}
}

func TestWizardRun_DetectsLanguage(t *testing.T) {
	dir := t.TempDir()

	// Create go.mod in temp dir
	gomod := "module github.com/test/project\n\ngo 1.21\n"
	if err := writeFile(dir, "go.mod", gomod); err != nil {
		t.Fatal(err)
	}

	input := "Build a CLI tool\n\n\n"
	stdin := strings.NewReader(input)
	stdout := &bytes.Buffer{}

	w := &Wizard{
		Dir:    dir,
		Stdin:  stdin,
		Stdout: stdout,
	}

	result, err := w.Run()
	if err != nil {
		t.Fatalf("Wizard.Run() error: %v", err)
	}

	if result.Language != "go" {
		t.Errorf("Language = %q, want %q", result.Language, "go")
	}

	if result.ProjectName != "project" {
		t.Errorf("ProjectName = %q, want %q", result.ProjectName, "project")
	}
}

func TestWizardRun_PrintsDetectedEnvironment(t *testing.T) {
	dir := t.TempDir()

	input := "A simple app\n\n\n"
	stdin := strings.NewReader(input)
	stdout := &bytes.Buffer{}

	w := &Wizard{
		Dir:    dir,
		Stdin:  stdin,
		Stdout: stdout,
	}

	_, err := w.Run()
	if err != nil {
		t.Fatalf("Wizard.Run() error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Detected") || !strings.Contains(output, "Language") {
		t.Errorf("output should contain detection info, got: %s", output)
	}
}

func TestWizardRun_RequiresDescription(t *testing.T) {
	dir := t.TempDir()

	// Empty description then a real one
	input := "\nA real description\n\n\n"
	stdin := strings.NewReader(input)
	stdout := &bytes.Buffer{}

	w := &Wizard{
		Dir:    dir,
		Stdin:  stdin,
		Stdout: stdout,
	}

	result, err := w.Run()
	if err != nil {
		t.Fatalf("Wizard.Run() error: %v", err)
	}

	if result.Description != "A real description" {
		t.Errorf("Description = %q, want %q", result.Description, "A real description")
	}
}

func TestWizardResult_HasDefaults(t *testing.T) {
	dir := t.TempDir()

	input := "My app\n\n\n"
	stdin := strings.NewReader(input)
	stdout := &bytes.Buffer{}

	w := &Wizard{
		Dir:    dir,
		Stdin:  stdin,
		Stdout: stdout,
	}

	result, err := w.Run()
	if err != nil {
		t.Fatalf("Wizard.Run() error: %v", err)
	}

	if result.Agent == "" {
		t.Error("Agent should have a default value")
	}

	if result.Network == "" {
		t.Error("Network should have a default value")
	}
}

// writeFileToDir is a test helper for creating files in a directory.
func writeFileToDir(dir, name, content string) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
}

// helper
func writeFile(dir, name, content string) error {
	return writeFileToDir(dir, name, content)
}
