package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewAgent(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"claude", false},
		{"claude-cli", false},
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
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

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

func TestGetAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		agent    string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "claude with ANTHROPIC_API_KEY",
			agent:    "claude",
			envVars:  map[string]string{"ANTHROPIC_API_KEY": "sk-ant-123"},
			expected: "sk-ant-123",
		},
		{
			name:     "claude without key",
			agent:    "claude",
			envVars:  map[string]string{},
			expected: "",
		},
		{
			name:     "amp with AMP_API_KEY",
			agent:    "amp",
			envVars:  map[string]string{"AMP_API_KEY": "amp-key-456"},
			expected: "amp-key-456",
		},
		{
			name:     "amp without key",
			agent:    "amp",
			envVars:  map[string]string{},
			expected: "",
		},
		{
			name:     "aider with OPENAI_API_KEY",
			agent:    "aider",
			envVars:  map[string]string{"OPENAI_API_KEY": "sk-openai-789"},
			expected: "sk-openai-789",
		},
		{
			name:     "aider fallback to ANTHROPIC_API_KEY",
			agent:    "aider",
			envVars:  map[string]string{"ANTHROPIC_API_KEY": "sk-ant-fallback"},
			expected: "sk-ant-fallback",
		},
		{
			name:     "aider prefers OPENAI_API_KEY over ANTHROPIC_API_KEY",
			agent:    "aider",
			envVars:  map[string]string{"OPENAI_API_KEY": "sk-openai", "ANTHROPIC_API_KEY": "sk-ant"},
			expected: "sk-openai",
		},
		{
			name:     "aider without any key",
			agent:    "aider",
			envVars:  map[string]string{},
			expected: "",
		},
		{
			name:     "claude-cli returns empty",
			agent:    "claude-cli",
			envVars:  map[string]string{},
			expected: "",
		},
		{
			name:     "unknown agent",
			agent:    "unknown",
			envVars:  map[string]string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"ANTHROPIC_API_KEY", "AMP_API_KEY", "OPENAI_API_KEY"} {
				t.Setenv(key, "")
			}
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			result := GetAPIKey(tt.agent)
			if result != tt.expected {
				t.Errorf("GetAPIKey(%q) = %q, want %q", tt.agent, result, tt.expected)
			}
		})
	}
}

func TestValidateAPIKey(t *testing.T) {
	tests := []struct {
		name        string
		agent       string
		envVars     map[string]string
		wantErr     bool
		errContains string
	}{
		{
			name:    "claude with key set",
			agent:   "claude",
			envVars: map[string]string{"ANTHROPIC_API_KEY": "sk-ant-123"},
			wantErr: false,
		},
		{
			name:        "claude without key",
			agent:       "claude",
			envVars:     map[string]string{},
			wantErr:     true,
			errContains: "ANTHROPIC_API_KEY",
		},
		{
			name:    "amp with key set",
			agent:   "amp",
			envVars: map[string]string{"AMP_API_KEY": "amp-key"},
			wantErr: false,
		},
		{
			name:        "amp without key",
			agent:       "amp",
			envVars:     map[string]string{},
			wantErr:     true,
			errContains: "AMP_API_KEY",
		},
		{
			name:    "aider with OPENAI key",
			agent:   "aider",
			envVars: map[string]string{"OPENAI_API_KEY": "sk-openai"},
			wantErr: false,
		},
		{
			name:    "aider with ANTHROPIC fallback",
			agent:   "aider",
			envVars: map[string]string{"ANTHROPIC_API_KEY": "sk-ant"},
			wantErr: false,
		},
		{
			name:        "aider without any key",
			agent:       "aider",
			envVars:     map[string]string{},
			wantErr:     true,
			errContains: "OPENAI_API_KEY or ANTHROPIC_API_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"ANTHROPIC_API_KEY", "AMP_API_KEY", "OPENAI_API_KEY"} {
				t.Setenv(key, "")
			}
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			err := ValidateAPIKey(tt.agent)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAPIKey(%q) error = %v, wantErr %v", tt.agent, err, tt.wantErr)
			}
			if err != nil && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}

func TestAmpAgentParseOutput(t *testing.T) {
	ag := NewAmpAgent()

	tests := []struct {
		name      string
		output    string
		completed bool
		success   bool
	}{
		{
			name:      "completed task",
			output:    "Done <promise>COMPLETE</promise>",
			completed: true,
			success:   true,
		},
		{
			name:      "incomplete task",
			output:    "Working on it",
			completed: false,
			success:   true,
		},
		{
			name:      "error in output",
			output:    "Error: compilation failed",
			completed: false,
			success:   false,
		},
		{
			name:      "lowercase error",
			output:    "error: missing file",
			completed: false,
			success:   false,
		},
		{
			name:      "completed with error",
			output:    "Error: minor issue <promise>COMPLETE</promise>",
			completed: true,
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

func TestAiderAgentParseOutput(t *testing.T) {
	ag := NewAiderAgent()

	tests := []struct {
		name      string
		output    string
		completed bool
		success   bool
	}{
		{
			name:      "completed task",
			output:    "Changes applied <promise>COMPLETE</promise>",
			completed: true,
			success:   true,
		},
		{
			name:      "incomplete task",
			output:    "Analyzing code",
			completed: false,
			success:   true,
		},
		{
			name:      "error in output",
			output:    "Error: file not found",
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

func TestStopSignal(t *testing.T) {
	tests := []struct {
		name     string
		agent    Agent
		expected string
	}{
		{"claude", NewClaudeAgent(), "<promise>COMPLETE</promise>"},
		{"claude-cli", NewClaudeCLIAgent(), "<promise>COMPLETE</promise>"},
		{"amp", NewAmpAgent(), "<promise>COMPLETE</promise>"},
		{"aider", NewAiderAgent(), "<promise>COMPLETE</promise>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.agent.StopSignal(); got != tt.expected {
				t.Errorf("StopSignal() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestClaudeAgentName(t *testing.T) {
	ag := NewClaudeAgent()
	if ag.Name() != "claude" {
		t.Errorf("expected name claude, got %s", ag.Name())
	}
}

func TestAmpAgentEnvironment(t *testing.T) {
	for _, key := range []string{"AMP_API_KEY", "ANTHROPIC_API_KEY"} {
		t.Setenv(key, "")
	}

	t.Setenv("AMP_API_KEY", "amp-test-key")
	t.Setenv("ANTHROPIC_API_KEY", "ant-test-key")

	ag := NewAmpAgent()
	env := ag.Environment()

	foundAmp := false
	foundAnt := false
	for _, e := range env {
		if e == "AMP_API_KEY=amp-test-key" {
			foundAmp = true
		}
		if e == "ANTHROPIC_API_KEY=ant-test-key" {
			foundAnt = true
		}
	}

	if !foundAmp {
		t.Error("expected AMP_API_KEY in environment")
	}
	if !foundAnt {
		t.Error("expected ANTHROPIC_API_KEY in environment")
	}
}

func TestAiderAgentEnvironment(t *testing.T) {
	for _, key := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY"} {
		t.Setenv(key, "")
	}

	t.Setenv("OPENAI_API_KEY", "openai-test-key")
	t.Setenv("ANTHROPIC_API_KEY", "ant-test-key")

	ag := NewAiderAgent()
	env := ag.Environment()

	foundOpenAI := false
	foundAnt := false
	for _, e := range env {
		if e == "OPENAI_API_KEY=openai-test-key" {
			foundOpenAI = true
		}
		if e == "ANTHROPIC_API_KEY=ant-test-key" {
			foundAnt = true
		}
	}

	if !foundOpenAI {
		t.Error("expected OPENAI_API_KEY in environment")
	}
	if !foundAnt {
		t.Error("expected ANTHROPIC_API_KEY in environment")
	}
}

func TestAmpAgentCommandWithPrompt(t *testing.T) {
	ag := NewAmpAgent()

	cmd := ag.Command("")
	if len(cmd) != 1 {
		t.Errorf("expected 1 arg for empty prompt, got %d: %v", len(cmd), cmd)
	}

	cmd = ag.Command("fix the bug")
	found := false
	for i, arg := range cmd {
		if arg == "--message" && i+1 < len(cmd) && cmd[i+1] == "fix the bug" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --message flag with prompt in args: %v", cmd)
	}
}

func TestAiderAgentCommandWithPrompt(t *testing.T) {
	ag := NewAiderAgent()

	cmd := ag.Command("")
	// Should have base args but no message
	for _, arg := range cmd {
		if arg == "--message" {
			t.Error("expected no --message flag for empty prompt")
		}
	}

	cmd = ag.Command("add tests")
	found := false
	for i, arg := range cmd {
		if arg == "--message" && i+1 < len(cmd) && cmd[i+1] == "add tests" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --message flag with prompt in args: %v", cmd)
	}

	// Check aider-specific flags
	hasYes := false
	hasNoGit := false
	for _, arg := range cmd {
		if arg == "--yes" {
			hasYes = true
		}
		if arg == "--no-git" {
			hasNoGit = true
		}
	}
	if !hasYes {
		t.Error("expected --yes flag in aider command")
	}
	if !hasNoGit {
		t.Error("expected --no-git flag in aider command")
	}
}

func TestClaudeCLIAgentName(t *testing.T) {
	ag := NewClaudeCLIAgent()
	if ag.Name() != "claude-cli" {
		t.Errorf("expected name claude-cli, got %s", ag.Name())
	}
}

func TestClaudeCLIAgentCommand(t *testing.T) {
	ag := NewClaudeCLIAgent()

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

func TestClaudeCLIAgentEnvironment(t *testing.T) {
	ag := NewClaudeCLIAgent()
	env := ag.Environment()

	for _, e := range env {
		if strings.HasPrefix(e, "ANTHROPIC_API_KEY=") {
			t.Error("claude-cli should not set ANTHROPIC_API_KEY")
		}
	}

	foundHome := false
	for _, e := range env {
		if e == "HOME=/home/agent" {
			foundHome = true
		}
	}
	if !foundHome {
		t.Error("expected HOME=/home/agent in environment")
	}
}

func TestClaudeCLIAgentParseOutput(t *testing.T) {
	ag := NewClaudeCLIAgent()

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

func TestValidateAPIKeyClaudeCLI(t *testing.T) {
	// Test with a valid ~/.claude/ directory using a temp dir
	tmpHome := t.TempDir()
	claudeDir := filepath.Join(tmpHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", tmpHome)

	err := ValidateAPIKey("claude-cli")
	if err != nil {
		t.Errorf("ValidateAPIKey(claude-cli) with ~/.claude/ dir should not error, got %v", err)
	}

	// Test with missing ~/.claude/ directory
	tmpHomeEmpty := t.TempDir()
	t.Setenv("HOME", tmpHomeEmpty)

	err = ValidateAPIKey("claude-cli")
	if err == nil {
		t.Error("ValidateAPIKey(claude-cli) without ~/.claude/ dir should error")
	}
	if err != nil && !strings.Contains(err.Error(), "claude login") {
		t.Errorf("error should mention 'claude login', got %q", err.Error())
	}
}
