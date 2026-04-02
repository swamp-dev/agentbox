package container

import (
	"os"
	"testing"

	"github.com/swamp-dev/agentbox/internal/config"
)

func TestImageName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"node", "agentbox/node:20"},
		{"python", "agentbox/python:3.12"},
		{"go", "agentbox/go:1.22"},
		{"rust", "agentbox/rust:1.77"},
		{"full", "agentbox/full:latest"},
		{"custom:tag", "custom:tag"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ImageName(tt.input)
			if result != tt.expected {
				t.Errorf("ImageName(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseMemory(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		wantErr  bool
	}{
		{"4g", 4 * 1024 * 1024 * 1024, false},
		{"512m", 512 * 1024 * 1024, false},
		{"1024k", 1024 * 1024, false},
		{"1024", 1024, false},
		{"", 0, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseMemory(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMemory(%s) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if result != tt.expected {
				t.Errorf("ParseMemory(%s) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseCPUs(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
		wantErr  bool
	}{
		{"2", 2.0, false},
		{"0.5", 0.5, false},
		{"1.5", 1.5, false},
		{"", 0, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseCPUs(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCPUs(%s) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if result != tt.expected {
				t.Errorf("ParseCPUs(%s) = %f, want %f", tt.input, result, tt.expected)
			}
		})
	}
}

func TestConfigToContainerConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Project.Name = "test-project"
	cfg.Docker.Image = "node"
	cfg.Docker.Resources.Memory = "2g"
	cfg.Docker.Resources.CPUs = "1"
	cfg.Docker.Network = "none"

	projectDir := t.TempDir()

	containerCfg, err := ConfigToContainerConfig(cfg, projectDir, []string{"claude"}, []string{"KEY=value"})
	if err != nil {
		t.Fatalf("ConfigToContainerConfig() error = %v", err)
	}

	if containerCfg.Image != "agentbox/node:20" {
		t.Errorf("expected image agentbox/node:20, got %s", containerCfg.Image)
	}

	if containerCfg.Memory != 2*1024*1024*1024 {
		t.Errorf("expected memory 2GB, got %d", containerCfg.Memory)
	}

	if containerCfg.CPUs != 1.0 {
		t.Errorf("expected 1 CPU, got %f", containerCfg.CPUs)
	}

	if containerCfg.Network != "none" {
		t.Errorf("expected network none, got %s", containerCfg.Network)
	}

	if len(containerCfg.Cmd) != 1 || containerCfg.Cmd[0] != "claude" {
		t.Errorf("expected cmd [claude], got %v", containerCfg.Cmd)
	}

	if len(containerCfg.Env) != 1 || containerCfg.Env[0] != "KEY=value" {
		t.Errorf("expected env [KEY=value], got %v", containerCfg.Env)
	}
}

func TestConfigToContainerConfigInvalidMemory(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Docker.Resources.Memory = "invalid"

	projectDir := t.TempDir()
	_, err := ConfigToContainerConfig(cfg, projectDir, nil, nil)
	if err == nil {
		t.Error("expected error for invalid memory")
	}
}

func TestConfigToContainerConfigInvalidCPU(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Docker.Resources.CPUs = "invalid"

	projectDir := t.TempDir()
	_, err := ConfigToContainerConfig(cfg, projectDir, nil, nil)
	if err == nil {
		t.Error("expected error for invalid CPU")
	}
}

func TestValidateProjectPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid temp dir", t.TempDir(), false},
		{"nonexistent", "/nonexistent/path/12345", true},
		{"etc blocked", "/etc", true},
		{"etc subdir blocked", "/etc/nginx", true},
		{"root exact blocked", "/root", true},
		{"proc blocked", "/proc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProjectPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProjectPath(%s) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}

	// /root subdirectories should be allowed (path must exist on disk)
	t.Run("root subdir allowed", func(t *testing.T) {
		subdir := t.TempDir() // creates under /tmp, but test the logic directly
		// Create a real subdir under /root if we're running as root
		if home, err := os.UserHomeDir(); err == nil && home == "/root" {
			testDir := "/root/agentbox-test-validate"
			if err := os.MkdirAll(testDir, 0o755); err == nil {
				defer os.Remove(testDir)
				if err := ValidateProjectPath(testDir); err != nil {
					t.Errorf("ValidateProjectPath(%s) should be allowed for /root subdirs, got: %v", testDir, err)
				}
				return
			}
		}
		// Fallback: just verify temp dir works
		if err := ValidateProjectPath(subdir); err != nil {
			t.Errorf("ValidateProjectPath(%s) error = %v", subdir, err)
		}
	})
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello world", "hello world"},
		{"cursor show", "\x1b[?25hhello", "hello"},
		{"color codes", "\x1b[32mgreen\x1b[0m", "green"},
		{"mixed", "before\x1b[1;31mred\x1b[0mafter", "beforeredafter"},
		{"empty", "", ""},
		{"no escapes", "just plain text\nwith newlines\n", "just plain text\nwith newlines\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSI(tt.input)
			if got != tt.want {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
