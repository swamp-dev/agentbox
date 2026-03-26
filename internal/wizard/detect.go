// Package wizard provides environment detection and interactive setup for agentbox projects.
package wizard

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/swamp-dev/agentbox/internal/config"
)

// AgentInfo describes an available AI agent.
type AgentInfo struct {
	Name       string
	AuthMethod string
	Available  bool
}

// DetectLanguage inspects the directory for manifest files and returns
// the project language: "node", "go", "rust", "python", or "full".
func DetectLanguage(dir string) string {
	// Check in priority order
	manifests := []struct {
		file string
		lang string
	}{
		{"package.json", "node"},
		{"go.mod", "go"},
		{"Cargo.toml", "rust"},
		{"pyproject.toml", "python"},
		{"requirements.txt", "python"},
	}

	for _, m := range manifests {
		if _, err := os.Stat(filepath.Join(dir, m.file)); err == nil {
			return m.lang
		}
	}

	return "full"
}

// DetectProjectName tries to read the project name from manifest files,
// falling back to the directory basename.
func DetectProjectName(dir string) string {
	// Try package.json
	if data, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		var pkg struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(data, &pkg); err == nil && pkg.Name != "" {
			return pkg.Name
		}
	}

	// Try go.mod
	if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "module ") {
				mod := strings.TrimPrefix(line, "module ")
				mod = strings.TrimSpace(mod)
				// Use last path component
				parts := strings.Split(mod, "/")
				if name := parts[len(parts)-1]; name != "" {
					return name
				}
			}
		}
	}

	return filepath.Base(dir)
}

// DetectAgents checks what AI agents are available on the system.
func DetectAgents() []AgentInfo {
	var agents []AgentInfo

	// Check claude CLI on PATH
	if _, err := exec.LookPath("claude"); err == nil {
		agents = append(agents, AgentInfo{
			Name:       "claude-cli",
			AuthMethod: "cli",
			Available:  true,
		})
	}

	// Check ANTHROPIC_API_KEY
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		agents = append(agents, AgentInfo{
			Name:       "claude",
			AuthMethod: "api_key",
			Available:  true,
		})
	}

	// Check AMP_API_KEY
	if os.Getenv("AMP_API_KEY") != "" {
		agents = append(agents, AgentInfo{
			Name:       "amp",
			AuthMethod: "api_key",
			Available:  true,
		})
	}

	// Check ~/.claude/ directory
	if home, err := os.UserHomeDir(); err == nil {
		if _, err := os.Stat(filepath.Join(home, ".claude")); err == nil {
			// Only add if claude-cli not already detected via PATH
			found := false
			for _, a := range agents {
				if a.Name == "claude-cli" {
					found = true
					break
				}
			}
			if !found {
				agents = append(agents, AgentInfo{
					Name:       "claude-cli",
					AuthMethod: "config_dir",
					Available:  true,
				})
			}
		}
	}

	// Check OPENAI_API_KEY
	if os.Getenv("OPENAI_API_KEY") != "" {
		agents = append(agents, AgentInfo{
			Name:       "aider",
			AuthMethod: "api_key",
			Available:  true,
		})
	}

	return agents
}

// DetectQualityChecks parses project files to find available quality check commands.
func DetectQualityChecks(dir, language string) []config.QualityCheck {
	switch language {
	case "node":
		return detectNodeQualityChecks(dir)
	case "go":
		return detectGoQualityChecks(dir)
	case "python":
		return detectPythonQualityChecks(dir)
	case "rust":
		return detectRustQualityChecks()
	default:
		return []config.QualityCheck{}
	}
}

func detectNodePackageManager(dir string) string {
	lockFiles := []struct {
		file    string
		manager string
	}{
		{"pnpm-lock.yaml", "pnpm run"},
		{"yarn.lock", "yarn run"},
		{"bun.lockb", "bun run"},
	}
	for _, lf := range lockFiles {
		if _, err := os.Stat(filepath.Join(dir, lf.file)); err == nil {
			return lf.manager
		}
	}
	return "npm run"
}

func detectNodeQualityChecks(dir string) []config.QualityCheck {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return []config.QualityCheck{}
	}

	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return []config.QualityCheck{}
	}

	runner := detectNodePackageManager(dir)
	relevantScripts := []string{"test", "typecheck", "lint", "build"}
	var checks []config.QualityCheck

	for _, name := range relevantScripts {
		if _, ok := pkg.Scripts[name]; ok {
			checks = append(checks, config.QualityCheck{
				Name:    name,
				Command: runner + " " + name,
			})
		}
	}

	return checks
}

func detectGoQualityChecks(dir string) []config.QualityCheck {
	data, err := os.ReadFile(filepath.Join(dir, "Makefile"))
	if err != nil {
		// Default go checks even without Makefile
		return []config.QualityCheck{
			{Name: "test", Command: "go test ./..."},
			{Name: "vet", Command: "go vet ./..."},
		}
	}

	relevantTargets := []string{"test", "lint", "vet"}
	var checks []config.QualityCheck

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	foundTargets := map[string]bool{}
	for scanner.Scan() {
		line := scanner.Text()
		for _, target := range relevantTargets {
			// Match "target:" at start of line
			if strings.HasPrefix(line, target+":") || strings.HasPrefix(line, target+" :") {
				foundTargets[target] = true
			}
		}
	}

	for _, target := range relevantTargets {
		if foundTargets[target] {
			checks = append(checks, config.QualityCheck{
				Name:    target,
				Command: "make " + target,
			})
		}
	}

	return checks
}

func detectPythonQualityChecks(dir string) []config.QualityCheck {
	var checks []config.QualityCheck

	data, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	if err != nil {
		return []config.QualityCheck{}
	}

	content := string(data)

	// Check for pytest
	if strings.Contains(content, "pytest") {
		checks = append(checks, config.QualityCheck{
			Name:    "test",
			Command: "python -m pytest",
		})
	}

	// Check for mypy
	if strings.Contains(content, "[tool.mypy]") {
		checks = append(checks, config.QualityCheck{
			Name:    "typecheck",
			Command: "python -m mypy .",
		})
	}

	// Check for ruff
	if strings.Contains(content, "[tool.ruff]") {
		checks = append(checks, config.QualityCheck{
			Name:    "lint",
			Command: "python -m ruff check .",
		})
	}

	return checks
}

func detectRustQualityChecks() []config.QualityCheck {
	return []config.QualityCheck{
		{Name: "test", Command: "cargo test"},
		{Name: "clippy", Command: "cargo clippy -- -D warnings"},
	}
}

// DetectDocker checks if Docker is available and running.
func DetectDocker() error {
	cmd := exec.Command("docker", "info")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}
