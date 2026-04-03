package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Version != "1.0" {
		t.Errorf("expected version 1.0, got %s", cfg.Version)
	}

	if cfg.Agent.Name != "claude" {
		t.Errorf("expected agent claude, got %s", cfg.Agent.Name)
	}

	if cfg.Docker.Network != "none" {
		t.Errorf("expected network none, got %s", cfg.Docker.Network)
	}

	if cfg.Ralph.MaxIterations != 10 {
		t.Errorf("expected max_iterations 10, got %d", cfg.Ralph.MaxIterations)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name            string
		modify          func(*Config)
		wantErr         bool
		wantErrContains string
	}{
		{
			name:    "valid default config",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name:            "invalid agent",
			modify:          func(c *Config) { c.Agent.Name = "invalid" },
			wantErr:         true,
			wantErrContains: "invalid agent",
		},
		{
			name:            "invalid image",
			modify:          func(c *Config) { c.Docker.Image = "invalid" },
			wantErr:         true,
			wantErrContains: "invalid image",
		},
		{
			name:            "invalid network",
			modify:          func(c *Config) { c.Docker.Network = "invalid" },
			wantErr:         true,
			wantErrContains: "invalid network",
		},
		{
			name:            "zero max iterations",
			modify:          func(c *Config) { c.Ralph.MaxIterations = 0 },
			wantErr:         true,
			wantErrContains: "max_iterations",
		},
		{
			name:    "valid amp agent",
			modify:  func(c *Config) { c.Agent.Name = "amp" },
			wantErr: false,
		},
		{
			name:    "valid aider agent",
			modify:  func(c *Config) { c.Agent.Name = "aider" },
			wantErr: false,
		},
		{
			name:    "valid claude-cli agent",
			modify:  func(c *Config) { c.Agent.Name = "claude-cli" },
			wantErr: false,
		},
		// Supervisor validation: sprint fields must be >= 1
		{
			name: "sprint_size zero with supervisor configured",
			modify: func(c *Config) {
				c.Supervisor.SprintSize = 0
				c.Supervisor.MaxSprints = 3
				c.Supervisor.MaxConsecutiveFails = 2
			},
			wantErr:         true,
			wantErrContains: "sprint_size",
		},
		{
			name: "max_sprints zero with supervisor configured",
			modify: func(c *Config) {
				c.Supervisor.SprintSize = 5
				c.Supervisor.MaxSprints = 0
				c.Supervisor.MaxConsecutiveFails = 2
			},
			wantErr:         true,
			wantErrContains: "max_sprints",
		},
		{
			name: "max_consecutive_fails zero with supervisor configured",
			modify: func(c *Config) {
				c.Supervisor.SprintSize = 5
				c.Supervisor.MaxSprints = 3
				c.Supervisor.MaxConsecutiveFails = 0
			},
			wantErr:         true,
			wantErrContains: "max_consecutive_fails",
		},
		{
			name: "negative sprint_size",
			modify: func(c *Config) {
				c.Supervisor.SprintSize = -1
				c.Supervisor.MaxSprints = 3
				c.Supervisor.MaxConsecutiveFails = 2
			},
			wantErr:         true,
			wantErrContains: "sprint_size",
		},
		{
			name: "sprint_size=1 boundary valid",
			modify: func(c *Config) {
				c.Supervisor.SprintSize = 1
				c.Supervisor.MaxSprints = 1
				c.Supervisor.MaxConsecutiveFails = 1
			},
			wantErr: false,
		},
		{
			name: "valid supervisor config",
			modify: func(c *Config) {
				c.Supervisor.SprintSize = 5
				c.Supervisor.MaxSprints = 3
				c.Supervisor.MaxConsecutiveFails = 2
				c.Supervisor.ReviewAfter = "sprint"
			},
			wantErr: false,
		},
		// review_after validation
		{
			name: "review_after task is invalid",
			modify: func(c *Config) {
				c.Supervisor.SprintSize = 5
				c.Supervisor.MaxSprints = 3
				c.Supervisor.MaxConsecutiveFails = 2
				c.Supervisor.ReviewAfter = "task"
			},
			wantErr:         true,
			wantErrContains: "review_after",
		},
		{
			name: "review_after invalid value",
			modify: func(c *Config) {
				c.Supervisor.SprintSize = 5
				c.Supervisor.MaxSprints = 3
				c.Supervisor.MaxConsecutiveFails = 2
				c.Supervisor.ReviewAfter = "bogus"
			},
			wantErr:         true,
			wantErrContains: "review_after",
		},
		{
			name: "review_after pr is valid",
			modify: func(c *Config) {
				c.Supervisor.SprintSize = 5
				c.Supervisor.MaxSprints = 3
				c.Supervisor.MaxConsecutiveFails = 2
				c.Supervisor.ReviewAfter = "pr"
			},
			wantErr: false,
		},
		{
			name: "review_after empty is valid",
			modify: func(c *Config) {
				c.Supervisor.SprintSize = 5
				c.Supervisor.MaxSprints = 3
				c.Supervisor.MaxConsecutiveFails = 2
				c.Supervisor.ReviewAfter = ""
			},
			wantErr: false,
		},
		// budget_duration validation
		{
			name: "invalid budget_duration",
			modify: func(c *Config) {
				c.Supervisor.SprintSize = 5
				c.Supervisor.MaxSprints = 3
				c.Supervisor.MaxConsecutiveFails = 2
				c.Supervisor.BudgetDuration = "not-a-duration"
			},
			wantErr:         true,
			wantErrContains: "budget_duration",
		},
		{
			name: "valid budget_duration",
			modify: func(c *Config) {
				c.Supervisor.SprintSize = 5
				c.Supervisor.MaxSprints = 3
				c.Supervisor.MaxConsecutiveFails = 2
				c.Supervisor.BudgetDuration = "30m"
			},
			wantErr: false,
		},
		// escalation_method validation
		{
			name: "supervisor escalation_method github_issue is valid",
			modify: func(c *Config) {
				c.Supervisor.SprintSize = 5
				c.Supervisor.MaxSprints = 3
				c.Supervisor.MaxConsecutiveFails = 2
				c.Supervisor.EscalationMethod = "github_issue"
			},
			wantErr: false,
		},
		{
			name: "supervisor escalation_method file is valid",
			modify: func(c *Config) {
				c.Supervisor.SprintSize = 5
				c.Supervisor.MaxSprints = 3
				c.Supervisor.MaxConsecutiveFails = 2
				c.Supervisor.EscalationMethod = "file"
			},
			wantErr: false,
		},
		{
			name: "supervisor escalation_method none is valid",
			modify: func(c *Config) {
				c.Supervisor.SprintSize = 5
				c.Supervisor.MaxSprints = 3
				c.Supervisor.MaxConsecutiveFails = 2
				c.Supervisor.EscalationMethod = "none"
			},
			wantErr: false,
		},
		{
			name: "supervisor escalation_method invalid value",
			modify: func(c *Config) {
				c.Supervisor.SprintSize = 5
				c.Supervisor.MaxSprints = 3
				c.Supervisor.MaxConsecutiveFails = 2
				c.Supervisor.EscalationMethod = "email"
			},
			wantErr:         true,
			wantErrContains: "escalation_method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrContains != "" && err != nil {
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.wantErrContains)
				}
			}
		})
	}
}

func TestLoadAndSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentbox.yaml")

	cfg := DefaultConfig()
	cfg.Project.Name = "test-project"

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Project.Name != cfg.Project.Name {
		t.Errorf("expected project name %s, got %s", cfg.Project.Name, loaded.Project.Name)
	}
}

func TestLoadNonExistent(t *testing.T) {
	cfg, err := Load("/nonexistent/path/agentbox.yaml")
	if err != nil {
		t.Fatalf("Load() should not error for missing file, got %v", err)
	}

	if cfg.Agent.Name != "claude" {
		t.Errorf("expected default agent claude, got %s", cfg.Agent.Name)
	}
}

func TestFindConfigFile(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(dir, "agentbox.yaml")
	if err := os.WriteFile(configPath, []byte("version: '1.0'"), 0644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()

	if err := os.Chdir(subdir); err != nil {
		t.Fatal(err)
	}

	found, err := FindConfigFile()
	if err != nil {
		t.Fatalf("FindConfigFile() error = %v", err)
	}

	if found != configPath {
		t.Errorf("expected %s, got %s", configPath, found)
	}
}
