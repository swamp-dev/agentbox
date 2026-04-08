package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReadSessionState(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		status    string
		errMsg    string
	}{
		{"running", "sess-001", "running", ""},
		{"completed", "sess-002", "completed", ""},
		{"failed", "sess-003", "failed", "container exited with code 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			err := WriteSessionState(dir, tt.sessionID, tt.status, tt.errMsg)
			if err != nil {
				t.Fatalf("WriteSessionState() error = %v", err)
			}

			got, err := ReadSessionState(dir, tt.sessionID)
			if err != nil {
				t.Fatalf("ReadSessionState() error = %v", err)
			}

			if got.SessionID != tt.sessionID {
				t.Errorf("SessionID = %q, want %q", got.SessionID, tt.sessionID)
			}
			if got.Status != tt.status {
				t.Errorf("Status = %q, want %q", got.Status, tt.status)
			}
			if got.Error != tt.errMsg {
				t.Errorf("Error = %q, want %q", got.Error, tt.errMsg)
			}
		})
	}
}

func TestWriteSessionState_AtomicRename(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-atomic"

	if err := WriteSessionState(dir, sessionID, "running", ""); err != nil {
		t.Fatal(err)
	}

	// The .tmp file should not exist after a successful write.
	tmp := filepath.Join(sessionDir(dir, sessionID), "status.json.tmp")
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("temp file should not exist after atomic write, got err = %v", err)
	}

	// The target file should exist.
	target := filepath.Join(sessionDir(dir, sessionID), "status.json")
	if _, err := os.Stat(target); err != nil {
		t.Errorf("target file should exist, got err = %v", err)
	}
}

func TestReadSessionState_Missing(t *testing.T) {
	dir := t.TempDir()

	_, err := ReadSessionState(dir, "nonexistent")
	if err == nil {
		t.Error("ReadSessionState() should return error for missing file")
	}
}

func TestWriteSessionState_StatusTransitions(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-transition"

	// Write running state.
	if err := WriteSessionState(dir, sessionID, "running", ""); err != nil {
		t.Fatal(err)
	}

	got, _ := ReadSessionState(dir, sessionID)
	if got.Status != "running" {
		t.Fatalf("initial status = %q, want running", got.Status)
	}

	// Transition to completed.
	if err := WriteSessionState(dir, sessionID, "completed", ""); err != nil {
		t.Fatal(err)
	}

	got, _ = ReadSessionState(dir, sessionID)
	if got.Status != "completed" {
		t.Errorf("status after transition = %q, want completed", got.Status)
	}
}

func TestWriteSessionState_FailedWithError(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-fail"

	if err := WriteSessionState(dir, sessionID, "running", ""); err != nil {
		t.Fatal(err)
	}

	errMsg := "container exited with code 1: out of memory"
	if err := WriteSessionState(dir, sessionID, "failed", errMsg); err != nil {
		t.Fatal(err)
	}

	got, _ := ReadSessionState(dir, sessionID)
	if got.Status != "failed" {
		t.Errorf("Status = %q, want failed", got.Status)
	}
	if got.Error != errMsg {
		t.Errorf("Error = %q, want %q", got.Error, errMsg)
	}
}

func TestRemoveSessionState(t *testing.T) {
	dir := t.TempDir()
	sessionID := "sess-remove"

	if err := WriteSessionState(dir, sessionID, "completed", ""); err != nil {
		t.Fatal(err)
	}

	if err := removeSessionState(dir, sessionID); err != nil {
		t.Fatalf("removeSessionState() error = %v", err)
	}

	_, err := ReadSessionState(dir, sessionID)
	if err == nil {
		t.Error("ReadSessionState() should fail after removal")
	}
}

func TestSessionDir(t *testing.T) {
	got := sessionDir("/my/project", "abc-123")
	want := "/my/project/.agentbox/sessions/abc-123"
	if got != want {
		t.Errorf("sessionDir() = %q, want %q", got, want)
	}
}
