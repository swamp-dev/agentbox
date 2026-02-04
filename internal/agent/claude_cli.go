package agent

import "strings"

// ClaudeCLIAgent implements the Agent interface for Claude Code using
// subscription-based authentication (Pro/Max plan) instead of an API key.
// It runs the same claude binary but relies on ~/.claude/ credentials
// mounted into the container rather than ANTHROPIC_API_KEY.
type ClaudeCLIAgent struct{}

// NewClaudeCLIAgent creates a new Claude CLI agent adapter.
func NewClaudeCLIAgent() *ClaudeCLIAgent {
	return &ClaudeCLIAgent{}
}

// Name returns the agent identifier.
func (a *ClaudeCLIAgent) Name() string {
	return "claude-cli"
}

// Command returns the command to run Claude Code with a prompt.
// Uses --dangerously-skip-permissions for autonomous execution. This is safe
// because the Docker container provides the security boundary.
func (a *ClaudeCLIAgent) Command(prompt string) []string {
	args := []string{"claude", "--dangerously-skip-permissions"}
	if prompt != "" {
		args = append(args, "-p", prompt)
	}
	return args
}

// Environment returns the environment variables needed by Claude Code
// when using subscription auth. No ANTHROPIC_API_KEY is set.
func (a *ClaudeCLIAgent) Environment() []string {
	return []string{
		"CLAUDE_CODE_SKIP_INTRO=1",
		"HOME=/home/agent",
		"USER=agent",
	}
}

// StopSignal returns the signal that indicates Claude has completed its task.
func (a *ClaudeCLIAgent) StopSignal() string {
	return "<promise>COMPLETE</promise>"
}

// ParseOutput extracts structured information from Claude's output.
func (a *ClaudeCLIAgent) ParseOutput(output string) *AgentOutput {
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
