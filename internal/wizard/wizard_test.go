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

func TestGenerateFiles_CreatesAllFiles(t *testing.T) {
	dir := t.TempDir()
	result := &WizardResult{
		ProjectName:   "myproject",
		Language:      "go",
		Agent:         "claude",
		Description:   "A test project",
		Network:       "none",
	}

	if err := result.GenerateFiles(dir, false); err != nil {
		t.Fatalf("GenerateFiles: %v", err)
	}

	for _, name := range []string{"agentbox.yaml", "prd.json", "progress.txt", "AGENTS.md"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s to be created", name)
		}
	}
}

func TestGenerateFiles_SkipsExistingWithoutForce(t *testing.T) {
	dir := t.TempDir()
	result := &WizardResult{
		ProjectName: "myproject",
		Language:    "go",
		Agent:       "claude",
		Description: "A test project",
		Network:     "none",
	}

	// Create agentbox.yaml with sentinel content.
	sentinel := "sentinel: true\n"
	if err := os.WriteFile(filepath.Join(dir, "agentbox.yaml"), []byte(sentinel), 0644); err != nil {
		t.Fatal(err)
	}

	if err := result.GenerateFiles(dir, false); err != nil {
		t.Fatalf("GenerateFiles: %v", err)
	}

	// Existing file should not be overwritten.
	got, err := os.ReadFile(filepath.Join(dir, "agentbox.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != sentinel {
		t.Errorf("existing file was overwritten; got %q, want %q", string(got), sentinel)
	}
}

func TestGenerateFiles_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	result := &WizardResult{
		ProjectName: "myproject",
		Language:    "go",
		Agent:       "claude",
		Description: "A test project",
		Network:     "none",
	}

	// Create agentbox.yaml with sentinel content.
	sentinel := "sentinel: true\n"
	if err := os.WriteFile(filepath.Join(dir, "agentbox.yaml"), []byte(sentinel), 0644); err != nil {
		t.Fatal(err)
	}

	if err := result.GenerateFiles(dir, true); err != nil {
		t.Fatalf("GenerateFiles with force: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "agentbox.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) == sentinel {
		t.Error("force=true should have overwritten existing file")
	}
}

func TestGenerateProgress_ContainsProjectName(t *testing.T) {
	dir := t.TempDir()
	result := &WizardResult{ProjectName: "coolapp"}

	if err := result.generateProgress(dir, true); err != nil {
		t.Fatalf("generateProgress: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "progress.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "coolapp") {
		t.Errorf("progress.txt should contain project name; got %q", string(content))
	}
}

func TestGenerateAgentsMD_ContainsSections(t *testing.T) {
	dir := t.TempDir()

	if err := generateAgentsMD(dir, true); err != nil {
		t.Fatalf("generateAgentsMD: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range []string{"Conventions", "Patterns", "Gotchas", "Commands"} {
		if !strings.Contains(string(content), section) {
			t.Errorf("AGENTS.md missing section %q", section)
		}
	}
}

func TestGenerateConfig_UsesWizardResult(t *testing.T) {
	dir := t.TempDir()
	result := &WizardResult{
		ProjectName: "testproj",
		Agent:       "amp",
		Language:    "python",
		Network:     "bridge",
	}

	if err := result.generateConfig(dir, true); err != nil {
		t.Fatalf("generateConfig: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "agentbox.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	for _, want := range []string{"testproj", "amp", "python", "bridge"} {
		if !strings.Contains(s, want) {
			t.Errorf("agentbox.yaml missing %q; got:\n%s", want, s)
		}
	}
}

func TestGeneratePRD_ContainsDescription(t *testing.T) {
	dir := t.TempDir()
	result := &WizardResult{
		ProjectName: "mything",
		Description: "Build a spaceship",
	}

	if err := result.generatePRD(dir, true); err != nil {
		t.Fatalf("generatePRD: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "prd.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "Build a spaceship") {
		t.Errorf("prd.json missing description; got %q", string(content))
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
