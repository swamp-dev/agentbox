package agent

import (
	"os"
	"strings"
)

// AiderAgent implements the Agent interface for Aider.
type AiderAgent struct{}

// NewAiderAgent creates a new Aider agent adapter.
func NewAiderAgent() *AiderAgent {
	return &AiderAgent{}
}

// Name returns the agent identifier.
func (a *AiderAgent) Name() string {
	return "aider"
}

// Command returns the command to run Aider with a prompt.
func (a *AiderAgent) Command(prompt string) []string {
	args := []string{"aider", "--yes", "--no-git"}
	if prompt != "" {
		args = append(args, "--message", prompt)
	}
	return args
}

// Environment returns the environment variables needed by Aider.
func (a *AiderAgent) Environment() []string {
	env := []string{}

	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		env = append(env, "OPENAI_API_KEY="+key)
	}

	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		env = append(env, "ANTHROPIC_API_KEY="+key)
	}

	env = append(env, "HOME=/home/agent")
	env = append(env, "USER=agent")

	return env
}

// StopSignal returns the signal that indicates Aider has completed its task.
func (a *AiderAgent) StopSignal() string {
	return "<promise>COMPLETE</promise>"
}

// ParseOutput extracts structured information from Aider's output.
func (a *AiderAgent) ParseOutput(output string) *AgentOutput {
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

	return result
}
