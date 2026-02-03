// Package config handles agentbox configuration parsing and validation.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the agentbox.yaml configuration file.
type Config struct {
	Version    string           `yaml:"version"`
	Project    ProjectConfig    `yaml:"project"`
	Agent      AgentConfig      `yaml:"agent"`
	Docker     DockerConfig     `yaml:"docker"`
	Ralph      RalphConfig      `yaml:"ralph"`
	Supervisor SupervisorConfig `yaml:"supervisor,omitempty"`
}

// SupervisorConfig controls the autonomous sprint behavior.
type SupervisorConfig struct {
	SprintSize          int    `yaml:"sprint_size"`
	MaxSprints          int    `yaml:"max_sprints"`
	MaxConsecutiveFails int    `yaml:"max_consecutive_fails"`
	ReviewAgent         string `yaml:"review_agent"`
	FallbackAgent       string `yaml:"fallback_agent"`
	ReviewAfter         string `yaml:"review_after"`
	BudgetDuration      string `yaml:"budget_duration"`
	JournalEnabled      bool   `yaml:"journal_enabled"`
	ReviewEnabled       bool   `yaml:"review_enabled"`
	EscalationMethod    string `yaml:"escalation_method"`
}

// ProjectConfig holds project-level settings.
type ProjectConfig struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

// AgentConfig specifies which AI agent to use.
type AgentConfig struct {
	Name string `yaml:"name"` // claude, amp, aider
}

// DockerConfig controls container resources and networking.
type DockerConfig struct {
	Image     string          `yaml:"image"` // node, python, go, rust, full
	Resources ResourcesConfig `yaml:"resources"`
	Network   string          `yaml:"network"` // none, bridge, host
}

// ResourcesConfig sets container resource limits.
type ResourcesConfig struct {
	Memory string `yaml:"memory"`
	CPUs   string `yaml:"cpus"`
}

// RalphConfig controls the Ralph loop behavior.
type RalphConfig struct {
	MaxIterations int            `yaml:"max_iterations"`
	PRDFile       string         `yaml:"prd_file"`
	ProgressFile  string         `yaml:"progress_file"`
	AutoCommit    bool           `yaml:"auto_commit"`
	QualityChecks []QualityCheck `yaml:"quality_checks"`
	StopSignal    string         `yaml:"stop_signal"`
}

// QualityCheck defines a command to run after each iteration.
type QualityCheck struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Version: "1.0",
		Project: ProjectConfig{
			Name: "my-project",
			Path: ".",
		},
		Agent: AgentConfig{
			Name: "claude",
		},
		Docker: DockerConfig{
			Image: "full",
			Resources: ResourcesConfig{
				Memory: "4g",
				CPUs:   "2",
			},
			Network: "none",
		},
		Ralph: RalphConfig{
			MaxIterations: 10,
			PRDFile:       "prd.json",
			ProgressFile:  "progress.txt",
			AutoCommit:    true,
			StopSignal:    "<promise>COMPLETE</promise>",
		},
	}
}

// Load reads and parses the agentbox.yaml config file.
func Load(path string) (*Config, error) {
	if path == "" {
		path = "agentbox.yaml"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return cfg, nil
}

// Save writes the configuration to the specified path.
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	validAgents := map[string]bool{"claude": true, "amp": true, "aider": true}
	if !validAgents[c.Agent.Name] {
		return fmt.Errorf("invalid agent: %s (must be claude, amp, or aider)", c.Agent.Name)
	}

	validImages := map[string]bool{"node": true, "python": true, "go": true, "rust": true, "full": true}
	if !validImages[c.Docker.Image] {
		return fmt.Errorf("invalid image: %s", c.Docker.Image)
	}

	validNetworks := map[string]bool{"none": true, "bridge": true, "host": true}
	if !validNetworks[c.Docker.Network] {
		return fmt.Errorf("invalid network: %s (must be none, bridge, or host)", c.Docker.Network)
	}

	if c.Ralph.MaxIterations < 1 {
		return fmt.Errorf("max_iterations must be at least 1")
	}

	return nil
}

// FindConfigFile searches for agentbox.yaml in current and parent directories.
func FindConfigFile() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for dir := cwd; ; dir = filepath.Dir(dir) {
		configPath := filepath.Join(dir, "agentbox.yaml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}

		if dir == filepath.Dir(dir) {
			break
		}
	}

	return "", fmt.Errorf("agentbox.yaml not found in %s or parent directories", cwd)
}
