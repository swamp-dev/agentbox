package wizard

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/swamp-dev/agentbox/internal/config"
)

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
			return nil, fmt.Errorf("reading description: %w", scanner.Err())
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
				agent = choice
			}
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
func (r *WizardResult) GenerateFiles(dir string) error {
	if err := r.generateConfig(dir); err != nil {
		return err
	}
	if err := r.generatePRD(dir); err != nil {
		return err
	}
	if err := r.generateProgress(dir); err != nil {
		return err
	}
	if err := generateAgentsMD(dir); err != nil {
		return err
	}
	return nil
}

func (r *WizardResult) generateConfig(dir string) error {
	path := dir + "/agentbox.yaml"
	cfg := config.DefaultConfig()
	cfg.Project.Name = r.ProjectName
	cfg.Agent.Name = r.Agent
	cfg.Docker.Image = r.Language
	cfg.Docker.Network = r.Network
	cfg.Ralph.QualityChecks = r.QualityChecks
	return cfg.Save(path)
}

func (r *WizardResult) generatePRD(dir string) error {
	path := dir + "/prd.json"
	prd := GeneratePRD(r.ProjectName, r.Description)
	return prd.Save(path)
}

func (r *WizardResult) generateProgress(dir string) error {
	path := dir + "/progress.txt"
	content := fmt.Sprintf("# Progress: %s\n\nNo iterations yet.\n", r.ProjectName)
	return os.WriteFile(path, []byte(content), 0644)
}

func generateAgentsMD(dir string) error {
	path := dir + "/AGENTS.md"
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

// writeFileToDir is a helper used by tests.
func writeFileToDir(dir, name, content string) error {
	return os.WriteFile(dir+"/"+name, []byte(content), 0644)
}
