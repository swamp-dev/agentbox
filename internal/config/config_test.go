package config

import (
	"os"
	"path/filepath"
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
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "valid default config",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name:    "invalid agent",
			modify:  func(c *Config) { c.Agent.Name = "invalid" },
			wantErr: true,
		},
		{
			name:    "invalid image",
			modify:  func(c *Config) { c.Docker.Image = "invalid" },
			wantErr: true,
		},
		{
			name:    "invalid network",
			modify:  func(c *Config) { c.Docker.Network = "invalid" },
			wantErr: true,
		},
		{
			name:    "zero max iterations",
			modify:  func(c *Config) { c.Ralph.MaxIterations = 0 },
			wantErr: true,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
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
	defer os.Chdir(origDir)

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
