package agent

import (
	"os"
	"strings"
)

// AmpAgent implements the Agent interface for Amp.
type AmpAgent struct{}

// NewAmpAgent creates a new Amp agent adapter.
func NewAmpAgent() *AmpAgent {
	return &AmpAgent{}
}

// Name returns the agent identifier.
func (a *AmpAgent) Name() string {
	return "amp"
}

// Command returns the command to run Amp with a prompt.
func (a *AmpAgent) Command(prompt string) []string {
	args := []string{"amp"}
	if prompt != "" {
		args = append(args, "--message", prompt)
	}
	return args
}

// Environment returns the environment variables needed by Amp.
func (a *AmpAgent) Environment() []string {
	env := []string{}

	if key := os.Getenv("AMP_API_KEY"); key != "" {
		env = append(env, "AMP_API_KEY="+key)
	}

	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		env = append(env, "ANTHROPIC_API_KEY="+key)
	}

	env = append(env, "HOME=/home/agent")
	env = append(env, "USER=agent")

	return env
}

// StopSignal returns the signal that indicates Amp has completed its task.
func (a *AmpAgent) StopSignal() string {
	return "<promise>COMPLETE</promise>"
}

// ParseOutput extracts structured information from Amp's output.
func (a *AmpAgent) ParseOutput(output string) *AgentOutput {
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
