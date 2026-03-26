package wizard

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/swamp-dev/agentbox/internal/config"
)

// knownAgents is the set of valid agent names.
var knownAgents = map[string]bool{
	"claude":     true,
	"claude-cli": true,
	"amp":        true,
	"aider":      true,
}

// Wizard orchestrates environment detection, user prompts, and file generation.
type Wizard struct {
	Dir    string
	Stdin  io.Reader
	Stdout io.Writer
}

// WizardResult holds the collected configuration from the wizard flow.
type WizardResult struct {
	ProjectName   string
	Language      string
	Agent         string
	QualityChecks []config.QualityCheck
	Description   string
	Network       string
}

// Run executes the interactive wizard flow:
// 1. Detect environment
// 2. Print detected settings
// 3. Ask required questions
// 4. Return result
func (w *Wizard) Run() (*WizardResult, error) {
	scanner := bufio.NewScanner(w.Stdin)

	// Detect environment
	language := DetectLanguage(w.Dir)
	projectName := DetectProjectName(w.Dir)
	agents := DetectAgents()
	qualityChecks := DetectQualityChecks(w.Dir, language)

	// Print detected environment
	fmt.Fprintf(w.Stdout, "\n  Detected environment:\n")
	fmt.Fprintf(w.Stdout, "    Language:       %s\n", language)
	fmt.Fprintf(w.Stdout, "    Project name:   %s\n", projectName)
	fmt.Fprintf(w.Stdout, "    Quality checks: %d found\n", len(qualityChecks))

	if len(agents) > 0 {
		names := make([]string, 0, len(agents))
		for _, a := range agents {
			if a.Available {
				names = append(names, a.Name)
			}
		}
		if len(names) > 0 {
			fmt.Fprintf(w.Stdout, "    Agents:         %s\n", strings.Join(names, ", "))
		}
	}
	fmt.Fprintln(w.Stdout)

	// Ask: What are you building? (required)
	var description string
	for description == "" {
		fmt.Fprint(w.Stdout, "  What are you building? ")
		if !scanner.Scan() {
			err := scanner.Err()
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			return nil, fmt.Errorf("reading description: %w", err)
		}
		description = strings.TrimSpace(scanner.Text())
		if description == "" {
			fmt.Fprintln(w.Stdout, "  (description is required)")
		}
	}

	// Pick agent
	agent := pickDefaultAgent(agents)
	if len(availableAgents(agents)) > 1 {
		fmt.Fprintf(w.Stdout, "  Which agent? [%s] ", agent)
		if scanner.Scan() {
			if choice := strings.TrimSpace(scanner.Text()); choice != "" {
				if knownAgents[choice] {
					agent = choice
				} else {
					fmt.Fprintf(w.Stdout, "  Unknown agent %q, using default %q\n", choice, agent)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("reading agent choice: %w", err)
		}
	}

	// If no agent detected at all, fall back to "claude"
	if agent == "" {
		agent = "claude"
	}

	// Network (default none)
	network := "none"
	fmt.Fprintf(w.Stdout, "  Network access? (none/bridge/host) [none] ")
	if scanner.Scan() {
		if choice := strings.TrimSpace(scanner.Text()); choice != "" {
			network = choice
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading network choice: %w", err)
	}

	return &WizardResult{
		ProjectName:   projectName,
		Language:      language,
		Agent:         agent,
		QualityChecks: qualityChecks,
		Description:   description,
		Network:       network,
	}, nil
}

// GenerateFiles creates agentbox project files from the wizard result.
// If force is false, existing files are skipped.
func (r *WizardResult) GenerateFiles(dir string, force bool) error {
	if err := r.generateConfig(dir, force); err != nil {
		return err
	}
	if err := r.generatePRD(dir, force); err != nil {
		return err
	}
	if err := r.generateProgress(dir, force); err != nil {
		return err
	}
	if err := generateAgentsMD(dir, force); err != nil {
		return err
	}
	return nil
}

func (r *WizardResult) generateConfig(dir string, force bool) error {
	path := filepath.Join(dir, "agentbox.yaml")
	if !force {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
	}
	cfg := config.DefaultConfig()
	cfg.Project.Name = r.ProjectName
	cfg.Agent.Name = r.Agent
	cfg.Docker.Image = r.Language
	cfg.Docker.Network = r.Network
	cfg.Ralph.QualityChecks = r.QualityChecks
	return cfg.Save(path)
}

func (r *WizardResult) generatePRD(dir string, force bool) error {
	path := filepath.Join(dir, "prd.json")
	if !force {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
	}
	prd := GeneratePRDTemplate(r.ProjectName, r.Description)
	return prd.Save(path)
}

func (r *WizardResult) generateProgress(dir string, force bool) error {
	path := filepath.Join(dir, "progress.txt")
	if !force {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
	}
	content := fmt.Sprintf("# Progress: %s\n\nNo iterations yet.\n", r.ProjectName)
	return os.WriteFile(path, []byte(content), 0644)
}

func generateAgentsMD(dir string, force bool) error {
	path := filepath.Join(dir, "AGENTS.md")
	if !force {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
	}
	content := `# AGENTS.md

This file documents patterns and learnings discovered by AI agents working on this project.

## Conventions

<!-- Document coding conventions discovered during development -->

## Patterns

<!-- Document recurring patterns in the codebase -->

## Gotchas

<!-- Document tricky areas or non-obvious behaviors -->

## Commands

<!-- Document useful commands for this project -->

## Notes

<!-- Additional notes from agent sessions -->
`
	return os.WriteFile(path, []byte(content), 0644)
}

func pickDefaultAgent(agents []AgentInfo) string {
	// Prefer claude-cli, then claude, then first available
	preferred := []string{"claude-cli", "claude", "amp", "aider"}
	for _, name := range preferred {
		for _, a := range agents {
			if a.Name == name && a.Available {
				return name
			}
		}
	}
	if len(agents) > 0 {
		return agents[0].Name
	}
	return ""
}

func availableAgents(agents []AgentInfo) []AgentInfo {
	var available []AgentInfo
	for _, a := range agents {
		if a.Available {
			available = append(available, a)
		}
	}
	return available
}
