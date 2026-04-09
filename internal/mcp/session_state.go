package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SessionState represents the on-disk state of an async session, used for
// cross-process communication between the MCP server and the wait command.
type SessionState struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"` // "running", "completed", "failed"
	Error     string `json:"error,omitempty"`
}

// sessionDir returns the directory for a session's state file.
func sessionDir(projectDir, sessionID string) string {
	return filepath.Join(projectDir, ".agentbox", "sessions", sessionID)
}

// WriteSessionState atomically writes a session state file to disk.
// Uses a temp file + rename to prevent partial reads.
// Exported for use by the CLI wait command tests.
func WriteSessionState(projectDir, sessionID, status, errMsg string) error {
	dir := sessionDir(projectDir, sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating session dir: %w", err)
	}

	state := SessionState{
		SessionID: sessionID,
		Status:    status,
		Error:     errMsg,
	}

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshaling session state: %w", err)
	}

	target := filepath.Join(dir, "status.json")
	tmp := target + ".tmp"

	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing temp state file: %w", err)
	}

	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming state file: %w", err)
	}

	return nil
}

// ReadSessionState reads a session state file from disk.
// Exported for use by the CLI wait command.
func ReadSessionState(projectDir, sessionID string) (*SessionState, error) {
	path := filepath.Join(sessionDir(projectDir, sessionID), "status.json")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading session state: %w", err)
	}

	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing session state: %w", err)
	}

	return &state, nil
}

// removeSessionState removes the session state directory for cleanup.
func removeSessionState(projectDir, sessionID string) error {
	return os.RemoveAll(sessionDir(projectDir, sessionID))
}
