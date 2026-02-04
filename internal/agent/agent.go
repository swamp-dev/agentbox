// Package agent provides adapters for different AI coding agents.
package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Agent defines the interface that all AI agent adapters must implement.
type Agent interface {
	// Name returns the agent identifier.
	Name() string

	// Command returns the command and arguments to run the agent.
	Command(prompt string) []string

	// Environment returns the environment variables needed by the agent.
	Environment() []string

	// StopSignal returns the signal that indicates the agent has completed its task.
	StopSignal() string

	// ParseOutput extracts structured information from agent output.
	ParseOutput(output string) *AgentOutput
}

// AgentOutput contains parsed information from an agent's execution.
type AgentOutput struct {
	Success   bool
	Completed bool
	Message   string
	Files     []string
}

// New creates an agent adapter by name.
func New(name string) (Agent, error) {
	switch strings.ToLower(name) {
	case "claude":
		return NewClaudeAgent(), nil
	case "amp":
		return NewAmpAgent(), nil
	case "aider":
		return NewAiderAgent(), nil
	case "claude-cli":
		return NewClaudeCLIAgent(), nil
	default:
		return nil, fmt.Errorf("unknown agent: %s", name)
	}
}

// GetAPIKey retrieves the API key for an agent from environment variables.
func GetAPIKey(agent string) string {
	switch agent {
	case "claude":
		return os.Getenv("ANTHROPIC_API_KEY")
	case "amp":
		return os.Getenv("AMP_API_KEY")
	case "aider":
		key := os.Getenv("OPENAI_API_KEY")
		if key == "" {
			key = os.Getenv("ANTHROPIC_API_KEY")
		}
		return key
	case "claude-cli":
		return ""
	default:
		return ""
	}
}

// ValidateAPIKey checks if the required API key or credentials are available.
func ValidateAPIKey(agent string) error {
	if agent == "claude-cli" {
		return validateClaudeCLICredentials()
	}

	key := GetAPIKey(agent)
	if key == "" {
		switch agent {
		case "claude":
			return fmt.Errorf("ANTHROPIC_API_KEY environment variable is required for Claude agent")
		case "amp":
			return fmt.Errorf("AMP_API_KEY environment variable is required for Amp agent")
		case "aider":
			return fmt.Errorf("OPENAI_API_KEY or ANTHROPIC_API_KEY environment variable is required for Aider agent")
		}
	}
	return nil
}

// validateClaudeCLICredentials checks that ~/.claude/ exists for subscription auth.
func validateClaudeCLICredentials() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	claudeDir := filepath.Join(home, ".claude")
	info, err := os.Stat(claudeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("~/.claude/ directory not found; run 'claude login' first to authenticate with your Claude subscription")
		}
		return fmt.Errorf("checking ~/.claude/ directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("~/.claude exists but is not a directory")
	}

	return nil
}
