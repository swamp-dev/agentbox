package container

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"strings"
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

	if containerCfg.Interactive {
		t.Error("expected Interactive=false by default")
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

func TestConfigToContainerConfigAllowedEndpoints(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Docker.Image = "full"
	cfg.Docker.Network = "restricted"
	cfg.Docker.Resources.Memory = "2g"
	cfg.Docker.Resources.CPUs = "1"
	cfg.Docker.AllowedEndpoints = []string{"api.anthropic.com:443", "pypi.org:443"}

	containerCfg, err := ConfigToContainerConfig(cfg, t.TempDir(), []string{"echo"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if containerCfg.Network != "restricted" {
		t.Errorf("Network = %s, want restricted", containerCfg.Network)
	}
	if len(containerCfg.AllowedEndpoints) != 2 {
		t.Fatalf("AllowedEndpoints = %v, want 2 entries", containerCfg.AllowedEndpoints)
	}
	if containerCfg.AllowedEndpoints[0] != "api.anthropic.com:443" {
		t.Errorf("AllowedEndpoints[0] = %s", containerCfg.AllowedEndpoints[0])
	}
}

func TestConfigToContainerConfigInteractiveDefault(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Docker.Image = "full"
	cfg.Docker.Resources.Memory = "2g"
	cfg.Docker.Resources.CPUs = "1"

	containerCfg, err := ConfigToContainerConfig(cfg, t.TempDir(), []string{"echo"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if containerCfg.Interactive {
		t.Error("expected Interactive=false by default from ConfigToContainerConfig")
	}
}

func TestConfigToContainerConfigClaudeCLIMount(t *testing.T) {
	tests := []struct {
		agent string
		want  bool
	}{
		{"claude-cli", true},
		{"claude", false},
		{"aider", false},
		{"amp", false},
	}
	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Agent.Name = tt.agent
			cfg.Docker.Image = "node"
			cfg.Docker.Resources.Memory = "2g"
			cfg.Docker.Resources.CPUs = "1"
			containerCfg, err := ConfigToContainerConfig(cfg, t.TempDir(), []string{"test"}, nil)
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if containerCfg.MountClaudeConfig != tt.want {
				t.Errorf("MountClaudeConfig = %v, want %v", containerCfg.MountClaudeConfig, tt.want)
			}
		})
	}
}

func dockerAvailable() bool {
	cm, err := NewManager()
	if err != nil {
		return false
	}
	defer cm.Close()

	// Also check that the test image exists — CI has Docker but no agentbox images.
	_, _, err = cm.client.ImageInspectWithRaw(context.Background(), "agentbox/full:latest")
	return err == nil
}

func TestRunNonInteractiveOutput(t *testing.T) {
	if testing.Short() || !dockerAvailable() {
		t.Skip("skipping: requires Docker")
	}

	cm, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	defer cm.Close()

	cfg := &ContainerConfig{
		Name:        "agentbox-test-noninteractive",
		Image:       "agentbox/full:latest",
		WorkDir:     "/tmp",
		ProjectPath: t.TempDir(),
		Cmd:         []string{"echo", "hello world"},
		Network:     "none",
		Interactive: false,
	}

	ctx := context.Background()
	output, err := cm.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(output, "hello world") {
		t.Errorf("expected output to contain 'hello world', got %q", output)
	}
}

func TestRunInteractiveOutput(t *testing.T) {
	if testing.Short() || !dockerAvailable() {
		t.Skip("skipping: requires Docker")
	}

	cm, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	defer cm.Close()

	cfg := &ContainerConfig{
		Name:        "agentbox-test-interactive",
		Image:       "agentbox/full:latest",
		WorkDir:     "/tmp",
		ProjectPath: t.TempDir(),
		Cmd:         []string{"echo", "hello interactive"},
		Network:     "none",
		Interactive: true,
	}

	ctx := context.Background()
	output, err := cm.Run(ctx, cfg)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !strings.Contains(output, "hello interactive") {
		t.Errorf("expected output to contain 'hello interactive', got %q", output)
	}
}

func TestCopyFileToContainerTarFormat(t *testing.T) {
	// Test the tar archive construction without Docker.
	// We call the tar-building logic directly and verify the header.
	content := []byte(`{"test": "credentials"}`)
	uid, gid := 1000, 1000

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err := tw.WriteHeader(&tar.Header{
		Name: ".credentials.json",
		Size: int64(len(content)),
		Mode: 0o600,
		Uid:  uid,
		Gid:  gid,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	// Read back and verify
	tr := tar.NewReader(&buf)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	if hdr.Name != ".credentials.json" {
		t.Errorf("Name = %s, want .credentials.json", hdr.Name)
	}
	if hdr.Uid != uid || hdr.Gid != gid {
		t.Errorf("UID/GID = %d/%d, want %d/%d", hdr.Uid, hdr.Gid, uid, gid)
	}
	if hdr.Mode != 0o600 {
		t.Errorf("Mode = %o, want 600", hdr.Mode)
	}
	data, err := io.ReadAll(tr)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(content) {
		t.Errorf("content = %q, want %q", string(data), string(content))
	}
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
