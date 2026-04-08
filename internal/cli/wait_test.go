package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/swamp-dev/agentbox/internal/mcp"
)

func TestRunWait_Completed(t *testing.T) {
	dir := t.TempDir()

	sessionID := "test-sess-completed"
	if err := mcp.WriteSessionState(dir, sessionID, "completed", ""); err != nil {
		t.Fatal(err)
	}

	// Configure the wait command flags.
	waitSession = sessionID
	waitProject = dir
	waitTimeout = 5 * time.Second
	waitPollInterval = 100 * time.Millisecond
	waitJSON = false

	// runWait should return nil for completed sessions.
	err := runWait(nil, nil)
	if err != nil {
		t.Errorf("runWait() error = %v, want nil", err)
	}
}

func TestRunWait_PollsUntilDone(t *testing.T) {
	dir := t.TempDir()
	sessionID := "test-sess-poll"

	// Write "running" initially.
	if err := mcp.WriteSessionState(dir, sessionID, "running", ""); err != nil {
		t.Fatal(err)
	}

	// Transition to "completed" after a short delay.
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = mcp.WriteSessionState(dir, sessionID, "completed", "")
	}()

	waitSession = sessionID
	waitProject = dir
	waitTimeout = 5 * time.Second
	waitPollInterval = 100 * time.Millisecond
	waitJSON = false

	err := runWait(nil, nil)
	if err != nil {
		t.Errorf("runWait() error = %v, want nil", err)
	}
}

func TestRunWait_JSON(t *testing.T) {
	dir := t.TempDir()
	sessionID := "test-sess-json"

	if err := mcp.WriteSessionState(dir, sessionID, "completed", ""); err != nil {
		t.Fatal(err)
	}

	waitSession = sessionID
	waitProject = dir
	waitTimeout = 5 * time.Second
	waitPollInterval = 100 * time.Millisecond
	waitJSON = true

	// Should not error.
	err := runWait(nil, nil)
	if err != nil {
		t.Errorf("runWait() error = %v, want nil", err)
	}
}

func TestSessionStateFileLocation(t *testing.T) {
	dir := t.TempDir()
	sessionID := "test-location"

	if err := mcp.WriteSessionState(dir, sessionID, "running", ""); err != nil {
		t.Fatal(err)
	}

	// Verify the file exists at the expected path.
	path := filepath.Join(dir, ".agentbox", "sessions", sessionID, "status.json")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("state file not found at expected path %s: %v", path, err)
	}
}
