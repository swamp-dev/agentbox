package cli

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	// Initialize the package-level logger used by CLI functions.
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	os.Exit(m.Run())
}

func TestCreateConfigFile(t *testing.T) {
	dir := t.TempDir()

	// Reset package-level vars.
	initForce = false
	initTemplate = "standard"
	initLanguage = ""

	if err := createConfigFile(dir, "test-project"); err != nil {
		t.Fatalf("createConfigFile() error: %v", err)
	}

	path := filepath.Join(dir, "agentbox.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected agentbox.yaml to be created")
	}

	// Second call without force should skip (not error).
	if err := createConfigFile(dir, "test-project"); err != nil {
		t.Fatalf("createConfigFile() second call should not error: %v", err)
	}

	// With force, should overwrite.
	initForce = true
	if err := createConfigFile(dir, "test-project"); err != nil {
		t.Fatalf("createConfigFile() with force should not error: %v", err)
	}
	initForce = false
}

func TestCreateConfigFile_MinimalTemplate(t *testing.T) {
	dir := t.TempDir()
	initForce = true
	initTemplate = "minimal"
	initLanguage = "python"
	defer func() {
		initForce = false
		initTemplate = "standard"
		initLanguage = ""
	}()

	if err := createConfigFile(dir, "minimal-project"); err != nil {
		t.Fatalf("createConfigFile() error: %v", err)
	}

	path := filepath.Join(dir, "agentbox.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty config file")
	}
}

func TestCreatePRDFile(t *testing.T) {
	dir := t.TempDir()
	initForce = false

	if err := createPRDFile(dir, "test-project"); err != nil {
		t.Fatalf("createPRDFile() error: %v", err)
	}

	path := filepath.Join(dir, "prd.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected prd.json to be created")
	}

	// Second call without force should skip.
	if err := createPRDFile(dir, "test-project"); err != nil {
		t.Fatalf("createPRDFile() second call should not error: %v", err)
	}

	// With force should overwrite.
	initForce = true
	if err := createPRDFile(dir, "test-project"); err != nil {
		t.Fatalf("createPRDFile() with force should not error: %v", err)
	}
	initForce = false
}

func TestCreateProgressFile(t *testing.T) {
	dir := t.TempDir()
	initForce = false

	if err := createProgressFile(dir, "test-project"); err != nil {
		t.Fatalf("createProgressFile() error: %v", err)
	}

	path := filepath.Join(dir, "progress.txt")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected progress.txt to be created")
	}

	// Second call without force should skip.
	if err := createProgressFile(dir, "test-project"); err != nil {
		t.Fatalf("createProgressFile() second call should not error: %v", err)
	}

	// With force should overwrite.
	initForce = true
	if err := createProgressFile(dir, "test-project"); err != nil {
		t.Fatalf("createProgressFile() with force should not error: %v", err)
	}
	initForce = false
}

func TestCreateAgentsMD(t *testing.T) {
	dir := t.TempDir()
	initForce = false

	if err := createAgentsMD(dir); err != nil {
		t.Fatalf("createAgentsMD() error: %v", err)
	}

	path := filepath.Join(dir, "AGENTS.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty AGENTS.md")
	}

	// Second call without force should skip.
	if err := createAgentsMD(dir); err != nil {
		t.Fatalf("createAgentsMD() second call should not error: %v", err)
	}

	// With force should overwrite.
	initForce = true
	if err := createAgentsMD(dir); err != nil {
		t.Fatalf("createAgentsMD() with force should not error: %v", err)
	}
	initForce = false
}

func TestRunNonInteractiveInit(t *testing.T) {
	dir := t.TempDir()

	// Set up package-level vars.
	initName = "test-init"
	initTemplate = "standard"
	initLanguage = ""
	initForce = true
	defer func() {
		initName = ""
		initTemplate = "standard"
		initLanguage = ""
		initForce = false
	}()

	if err := runNonInteractiveInit(dir); err != nil {
		t.Fatalf("runNonInteractiveInit() error: %v", err)
	}

	// Verify all files were created.
	for _, name := range []string{"agentbox.yaml", "prd.json", "progress.txt", "AGENTS.md"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected %s to be created", name)
		}
	}
}

func TestRunNonInteractiveInit_DefaultsNameFromDir(t *testing.T) {
	dir := t.TempDir()

	initName = "" // should default to directory name
	initTemplate = "standard"
	initLanguage = ""
	initForce = true
	defer func() {
		initName = ""
		initForce = false
	}()

	if err := runNonInteractiveInit(dir); err != nil {
		t.Fatalf("runNonInteractiveInit() error: %v", err)
	}

	// Verify config was created.
	path := filepath.Join(dir, "agentbox.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected agentbox.yaml to be created")
	}
}
