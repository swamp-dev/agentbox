package agent

import (
	"os"
	"testing"
)

func TestNewAgent(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"claude", false},
		{"amp", false},
		{"aider", false},
		{"invalid", true},
		{"Claude", false},
		{"CLAUDE", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ag, err := New(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("New(%s) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
			if !tt.wantErr && ag == nil {
				t.Errorf("New(%s) returned nil agent", tt.name)
			}
		})
	}
}

func TestClaudeAgentCommand(t *testing.T) {
	ag := NewClaudeAgent()

	cmd := ag.Command("")
	if len(cmd) < 2 {
		t.Errorf("expected at least 2 args, got %d", len(cmd))
	}
	if cmd[0] != "claude" {
		t.Errorf("expected claude command, got %s", cmd[0])
	}

	cmd = ag.Command("test prompt")
	found := false
	for i, arg := range cmd {
		if arg == "-p" && i+1 < len(cmd) && cmd[i+1] == "test prompt" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected prompt to be in command args")
	}
}

func TestClaudeAgentEnvironment(t *testing.T) {
	os.Setenv("ANTHROPIC_API_KEY", "test-key")
	defer os.Unsetenv("ANTHROPIC_API_KEY")

	ag := NewClaudeAgent()
	env := ag.Environment()

	found := false
	for _, e := range env {
		if e == "ANTHROPIC_API_KEY=test-key" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected ANTHROPIC_API_KEY in environment")
	}
}

func TestClaudeAgentParseOutput(t *testing.T) {
	ag := NewClaudeAgent()

	tests := []struct {
		name      string
		output    string
		completed bool
		success   bool
	}{
		{
			name:      "completed task",
			output:    "Task done <promise>COMPLETE</promise>",
			completed: true,
			success:   true,
		},
		{
			name:      "incomplete task",
			output:    "Still working on it",
			completed: false,
			success:   true,
		},
		{
			name:      "error in output",
			output:    "Error: something went wrong",
			completed: false,
			success:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ag.ParseOutput(tt.output)
			if result.Completed != tt.completed {
				t.Errorf("Completed = %v, want %v", result.Completed, tt.completed)
			}
			if result.Success != tt.success {
				t.Errorf("Success = %v, want %v", result.Success, tt.success)
			}
		})
	}
}

func TestAiderAgentCommand(t *testing.T) {
	ag := NewAiderAgent()

	if ag.Name() != "aider" {
		t.Errorf("expected name aider, got %s", ag.Name())
	}

	cmd := ag.Command("fix bug")
	if cmd[0] != "aider" {
		t.Errorf("expected aider command, got %s", cmd[0])
	}
}

func TestAmpAgentCommand(t *testing.T) {
	ag := NewAmpAgent()

	if ag.Name() != "amp" {
		t.Errorf("expected name amp, got %s", ag.Name())
	}

	cmd := ag.Command("add feature")
	if cmd[0] != "amp" {
		t.Errorf("expected amp command, got %s", cmd[0])
	}
}

func TestExtractFilePaths(t *testing.T) {
	output := `
Created src/main.go
Modified tests/main_test.go
Edited README.md
Other text here
`
	files := extractFilePaths(output)

	if len(files) != 3 {
		t.Errorf("expected 3 files, got %d: %v", len(files), files)
	}

	expected := []string{"src/main.go", "tests/main_test.go", "README.md"}
	for i, f := range expected {
		if files[i] != f {
			t.Errorf("expected %s, got %s", f, files[i])
		}
	}
}
