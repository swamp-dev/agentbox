package cli

import (
	"errors"
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

	err := runWait(waitConfig{
		session:      sessionID,
		project:      dir,
		timeout:      5 * time.Second,
		pollInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Errorf("runWait() error = %v, want nil", err)
	}
}

func TestRunWait_Failed(t *testing.T) {
	dir := t.TempDir()
	sessionID := "test-sess-failed"

	if err := mcp.WriteSessionState(dir, sessionID, "failed", "container crashed"); err != nil {
		t.Fatal(err)
	}

	err := runWait(waitConfig{
		session:      sessionID,
		project:      dir,
		timeout:      5 * time.Second,
		pollInterval: 100 * time.Millisecond,
	})
	if !errors.Is(err, ErrSessionFailed) {
		t.Errorf("runWait() error = %v, want ErrSessionFailed", err)
	}
}

func TestRunWait_Timeout(t *testing.T) {
	dir := t.TempDir()
	sessionID := "test-sess-timeout"

	// No state file written — will poll until timeout.
	err := runWait(waitConfig{
		session:      sessionID,
		project:      dir,
		timeout:      300 * time.Millisecond,
		pollInterval: 50 * time.Millisecond,
	})
	if !errors.Is(err, ErrWaitTimeout) {
		t.Errorf("runWait() error = %v, want ErrWaitTimeout", err)
	}
}

func TestRunWait_TimeoutWhileRunning(t *testing.T) {
	dir := t.TempDir()
	sessionID := "test-sess-running-timeout"

	// State file exists but stays "running" — should timeout.
	if err := mcp.WriteSessionState(dir, sessionID, "running", ""); err != nil {
		t.Fatal(err)
	}

	err := runWait(waitConfig{
		session:      sessionID,
		project:      dir,
		timeout:      300 * time.Millisecond,
		pollInterval: 50 * time.Millisecond,
	})
	if !errors.Is(err, ErrWaitTimeout) {
		t.Errorf("runWait() error = %v, want ErrWaitTimeout", err)
	}
}

func TestRunWait_PollsUntilDone(t *testing.T) {
	dir := t.TempDir()
	sessionID := "test-sess-poll"

	if err := mcp.WriteSessionState(dir, sessionID, "running", ""); err != nil {
		t.Fatal(err)
	}

	// Transition to "completed" after a short delay.
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = mcp.WriteSessionState(dir, sessionID, "completed", "")
	}()

	err := runWait(waitConfig{
		session:      sessionID,
		project:      dir,
		timeout:      5 * time.Second,
		pollInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Errorf("runWait() error = %v, want nil", err)
	}
}

func TestRunWait_NonExistentError(t *testing.T) {
	// Use a path that exists but has a permission issue simulated by
	// pointing to a non-directory file as the project dir.
	dir := t.TempDir()
	badPath := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(badPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// ReadSessionState will fail with a non-fs.ErrNotExist error
	// because it can't MkdirAll under a file.
	err := runWait(waitConfig{
		session:      "test-sess",
		project:      badPath,
		timeout:      1 * time.Second,
		pollInterval: 100 * time.Millisecond,
	})
	// Should return the underlying error, not timeout.
	if err == nil {
		t.Error("runWait() should return error for unreadable state")
	}
	if errors.Is(err, ErrWaitTimeout) {
		t.Error("runWait() should not timeout for non-existent errors — should fail fast")
	}
}

func TestRunWait_JSON(t *testing.T) {
	dir := t.TempDir()
	sessionID := "test-sess-json"

	if err := mcp.WriteSessionState(dir, sessionID, "completed", ""); err != nil {
		t.Fatal(err)
	}

	err := runWait(waitConfig{
		session:      sessionID,
		project:      dir,
		timeout:      5 * time.Second,
		pollInterval: 100 * time.Millisecond,
		jsonOutput:   true,
	})
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

	path := filepath.Join(dir, ".agentbox", "sessions", sessionID, "status.json")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("state file not found at expected path %s: %v", path, err)
	}
}
