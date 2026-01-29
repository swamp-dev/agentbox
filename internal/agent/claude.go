package agent

import (
	"os"
	"strings"
)

// ClaudeAgent implements the Agent interface for Claude Code.
type ClaudeAgent struct{}

// NewClaudeAgent creates a new Claude Code agent adapter.
func NewClaudeAgent() *ClaudeAgent {
	return &ClaudeAgent{}
}

// Name returns the agent identifier.
func (a *ClaudeAgent) Name() string {
	return "claude"
}

// Command returns the command to run Claude Code with a prompt.
// Uses --dangerously-skip-permissions for autonomous execution. This is safe
// because the Docker container provides the security boundary - the agent can
// only access the mounted /workspace directory.
func (a *ClaudeAgent) Command(prompt string) []string {
	args := []string{"claude", "--dangerously-skip-permissions"}
	if prompt != "" {
		args = append(args, "-p", prompt)
	}
	return args
}

// Environment returns the environment variables needed by Claude Code.
func (a *ClaudeAgent) Environment() []string {
	env := []string{}

	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		env = append(env, "ANTHROPIC_API_KEY="+key)
	}

	env = append(env, "CLAUDE_CODE_SKIP_INTRO=1")
	env = append(env, "HOME=/home/agent")
	env = append(env, "USER=agent")

	return env
}

// StopSignal returns the signal that indicates Claude has completed its task.
func (a *ClaudeAgent) StopSignal() string {
	return "<promise>COMPLETE</promise>"
}

// ParseOutput extracts structured information from Claude's output.
func (a *ClaudeAgent) ParseOutput(output string) *AgentOutput {
	result := &AgentOutput{
		Success: true,
		Message: output,
	}

	if strings.Contains(output, a.StopSignal()) {
		result.Completed = true
	}

	if strings.Contains(output, "Error:") || strings.Contains(output, "error:") {
		result.Success = false
	}

	result.Files = extractFilePaths(output)

	return result
}

// extractFilePaths attempts to find file paths mentioned in the output.
func extractFilePaths(output string) []string {
	var files []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Created ") || strings.HasPrefix(line, "Modified ") ||
			strings.HasPrefix(line, "Edited ") || strings.HasPrefix(line, "Wrote ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				path := parts[1]
				path = strings.Trim(path, "`'\"")
				if path != "" {
					files = append(files, path)
				}
			}
		}
	}

	return files
}
