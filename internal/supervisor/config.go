// Package supervisor provides the meta-orchestrator for the full development lifecycle.
package supervisor

import (
	"path/filepath"
	"time"

	"github.com/swamp-dev/agentbox/internal/config"
	"github.com/swamp-dev/agentbox/internal/metrics"
)

// Config holds supervisor-specific configuration.
type Config struct {
	// Sprint settings.
	SprintSize          int           `yaml:"sprint_size" json:"sprint_size"`
	MaxSprints          int           `yaml:"max_sprints" json:"max_sprints"`
	MaxConsecutiveFails int           `yaml:"max_consecutive_fails" json:"max_consecutive_fails"`

	// Agent settings.
	Agent         string `yaml:"agent" json:"agent"`
	ReviewAgent   string `yaml:"review_agent" json:"review_agent"`
	FallbackAgent string `yaml:"fallback_agent" json:"fallback_agent"`

	// Review settings.
	ReviewAfter    string `yaml:"review_after" json:"review_after"`       // "sprint", "task", "pr"
	MaxReviewRounds int   `yaml:"max_review_rounds" json:"max_review_rounds"`

	// Budget.
	Budget metrics.Budget `yaml:"budget" json:"budget"`

	// Features.
	JournalEnabled bool `yaml:"journal_enabled" json:"journal_enabled"`
	ReviewEnabled  bool `yaml:"review_enabled" json:"review_enabled"`
	AutoCommit     bool `yaml:"auto_commit" json:"auto_commit"`

	// Escalation.
	EscalationMethod string `yaml:"escalation_method" json:"escalation_method"` // "github_issue", "file", "none"

	// Paths.
	RepoURL    string `yaml:"repo_url" json:"repo_url"`
	PRDFile    string `yaml:"prd_file" json:"prd_file"`
	WorkDir    string `yaml:"work_dir" json:"work_dir"`
	BranchName string `yaml:"branch_name" json:"branch_name"`

	// Budget duration as string for YAML parsing.
	BudgetDuration string `yaml:"budget_duration" json:"budget_duration"`

	// Docker settings (used to build config.Config for ralph.Loop).
	DockerImage   string `yaml:"docker_image" json:"docker_image"`
	DockerMemory  string `yaml:"docker_memory" json:"docker_memory"`
	DockerCPUs    string `yaml:"docker_cpus" json:"docker_cpus"`
	DockerNetwork string `yaml:"docker_network" json:"docker_network"`

	// Quality checks run after each iteration.
	QualityChecks []QualityCheck `yaml:"quality_checks" json:"quality_checks"`

	// DryRun mode — use NoopAgentRunner instead of real agent.
	DryRun bool `yaml:"-" json:"-"`
}

// QualityCheck defines a command to run after each iteration.
type QualityCheck struct {
	Name    string `yaml:"name" json:"name"`
	Command string `yaml:"command" json:"command"`
}

// DefaultConfig returns a supervisor config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		SprintSize:          5,
		MaxSprints:          20,
		MaxConsecutiveFails: 3,
		Agent:               "claude",
		ReviewAgent:         "claude",
		ReviewAfter:         "sprint",
		MaxReviewRounds:     2,
		Budget:              metrics.DefaultBudget(),
		JournalEnabled:      true,
		ReviewEnabled:       true,
		AutoCommit:          true,
		EscalationMethod:    "file",
		PRDFile:             "prd.json",
		DockerImage:         "full",
		DockerMemory:        "4g",
		DockerCPUs:          "2",
		DockerNetwork:       "none",
	}
}

// ToRalphConfig builds a config.Config suitable for ralph.NewLoop().
func (c *Config) ToRalphConfig() *config.Config {
	workDir := c.WorkDir
	if workDir == "" {
		workDir = "."
	}
	return &config.Config{
		Version: "1.0",
		Project: config.ProjectConfig{Name: filepath.Base(workDir)},
		Agent:   config.AgentConfig{Name: c.Agent},
		Docker: config.DockerConfig{
			Image:     c.DockerImage,
			Resources: config.ResourcesConfig{Memory: c.DockerMemory, CPUs: c.DockerCPUs},
			Network:   c.DockerNetwork,
		},
		Ralph: config.RalphConfig{
			MaxIterations: c.MaxSprints * c.SprintSize,
			PRDFile:       c.PRDFile,
			ProgressFile:  "progress.txt",
			AutoCommit:    false, // Supervisor handles commits via SprintRunner.
			StopSignal:    "<promise>COMPLETE</promise>",
			QualityChecks: toConfigQualityChecks(c.QualityChecks),
		},
	}
}

// toConfigQualityChecks converts supervisor QualityChecks to config QualityChecks.
func toConfigQualityChecks(checks []QualityCheck) []config.QualityCheck {
	if len(checks) == 0 {
		return nil
	}
	out := make([]config.QualityCheck, len(checks))
	for i, qc := range checks {
		out[i] = config.QualityCheck{Name: qc.Name, Command: qc.Command}
	}
	return out
}

// ParseBudgetDuration parses the BudgetDuration string into the Budget.
func (c *Config) ParseBudgetDuration() error {
	if c.BudgetDuration != "" {
		d, err := time.ParseDuration(c.BudgetDuration)
		if err != nil {
			return err
		}
		c.Budget.MaxDuration = d
	}
	return nil
}
