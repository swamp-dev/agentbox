// Package container provides Docker container management for agentbox.
package container

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/swamp-dev/agentbox/internal/config"
)

// Manager handles Docker container lifecycle.
type Manager struct {
	client *client.Client
}

// NewManager creates a new Docker container manager.
func NewManager() (*Manager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("creating docker client: %w", err)
	}

	return &Manager{client: cli}, nil
}

// Close releases the Docker client resources.
func (m *Manager) Close() error {
	return m.client.Close()
}

// ContainerConfig holds all settings for creating a container.
type ContainerConfig struct {
	Name        string
	Image       string
	WorkDir     string
	ProjectPath string
	Env         []string
	Cmd         []string
	Network     string
	Memory      int64
	CPUs        float64
	MountSSH    bool
	MountGit    bool
}

// ImageName returns the full Docker image name for a given image type.
func ImageName(imageType string) string {
	switch imageType {
	case "node":
		return "agentbox/node:20"
	case "python":
		return "agentbox/python:3.12"
	case "go":
		return "agentbox/go:1.22"
	case "rust":
		return "agentbox/rust:1.77"
	case "full":
		return "agentbox/full:latest"
	default:
		return imageType
	}
}

// Create builds and starts a new container with the given configuration.
func (m *Manager) Create(ctx context.Context, cfg *ContainerConfig) (string, error) {
	mounts := []mount.Mount{
		{
			Type:   mount.TypeBind,
			Source: cfg.ProjectPath,
			Target: "/workspace",
		},
	}

	if cfg.MountSSH {
		home, _ := os.UserHomeDir()
		sshPath := filepath.Join(home, ".ssh")
		if _, err := os.Stat(sshPath); err == nil {
			mounts = append(mounts, mount.Mount{
				Type:     mount.TypeBind,
				Source:   sshPath,
				Target:   "/home/agent/.ssh",
				ReadOnly: true,
			})
		}
	}

	if cfg.MountGit {
		home, _ := os.UserHomeDir()
		gitConfig := filepath.Join(home, ".gitconfig")
		if _, err := os.Stat(gitConfig); err == nil {
			mounts = append(mounts, mount.Mount{
				Type:     mount.TypeBind,
				Source:   gitConfig,
				Target:   "/home/agent/.gitconfig",
				ReadOnly: true,
			})
		}
	}

	containerCfg := &container.Config{
		Image:      cfg.Image,
		Cmd:        cfg.Cmd,
		Env:        cfg.Env,
		WorkingDir: "/workspace",
		Tty:        true,
		OpenStdin:  true,
	}

	hostCfg := &container.HostConfig{
		Mounts: mounts,
		Resources: container.Resources{
			Memory:   cfg.Memory,
			NanoCPUs: int64(cfg.CPUs * 1e9),
		},
	}

	networkCfg := &network.NetworkingConfig{}
	if cfg.Network == "none" {
		hostCfg.NetworkMode = "none"
	} else if cfg.Network == "host" {
		hostCfg.NetworkMode = "host"
	}

	resp, err := m.client.ContainerCreate(ctx, containerCfg, hostCfg, networkCfg, nil, cfg.Name)
	if err != nil {
		return "", fmt.Errorf("creating container: %w", err)
	}

	if err := m.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("starting container: %w", err)
	}

	return resp.ID, nil
}

// Run creates a container, runs the command, and returns the output.
func (m *Manager) Run(ctx context.Context, cfg *ContainerConfig) (string, error) {
	containerID, err := m.Create(ctx, cfg)
	if err != nil {
		return "", err
	}
	defer func() { _ = m.Remove(ctx, containerID) }()

	return m.Wait(ctx, containerID)
}

// Wait blocks until the container exits and returns its output.
func (m *Manager) Wait(ctx context.Context, containerID string) (string, error) {
	statusCh, errCh := m.client.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)

	select {
	case err := <-errCh:
		if err != nil {
			return "", fmt.Errorf("waiting for container: %w", err)
		}
	case status := <-statusCh:
		if status.StatusCode != 0 {
			logs, _ := m.Logs(ctx, containerID)
			return logs, fmt.Errorf("container exited with code %d", status.StatusCode)
		}
	}

	return m.Logs(ctx, containerID)
}

// Logs retrieves the container's stdout and stderr.
func (m *Manager) Logs(ctx context.Context, containerID string) (string, error) {
	out, err := m.client.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return "", fmt.Errorf("getting container logs: %w", err)
	}
	defer out.Close()

	var stdout, stderr strings.Builder
	if _, err := stdcopy.StdCopy(&stdout, &stderr, out); err != nil {
		return "", fmt.Errorf("reading container logs: %w", err)
	}

	return stdout.String() + stderr.String(), nil
}

// Attach connects to a running container's stdin/stdout/stderr.
func (m *Manager) Attach(ctx context.Context, containerID string) error {
	resp, err := m.client.ContainerAttach(ctx, containerID, container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return fmt.Errorf("attaching to container: %w", err)
	}
	defer resp.Close()

	go func() {
		_, _ = io.Copy(os.Stdout, resp.Reader)
	}()

	_, err = io.Copy(resp.Conn, os.Stdin)
	return err
}

// Stop gracefully stops a running container.
func (m *Manager) Stop(ctx context.Context, containerID string) error {
	timeout := 10
	return m.client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

// Remove deletes a container.
func (m *Manager) Remove(ctx context.Context, containerID string) error {
	return m.client.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
}

// ParseMemory converts a memory string (e.g., "4g") to bytes.
func ParseMemory(mem string) (int64, error) {
	mem = strings.ToLower(strings.TrimSpace(mem))
	if mem == "" {
		return 0, nil
	}

	var multiplier int64 = 1
	if strings.HasSuffix(mem, "g") {
		multiplier = 1024 * 1024 * 1024
		mem = strings.TrimSuffix(mem, "g")
	} else if strings.HasSuffix(mem, "m") {
		multiplier = 1024 * 1024
		mem = strings.TrimSuffix(mem, "m")
	} else if strings.HasSuffix(mem, "k") {
		multiplier = 1024
		mem = strings.TrimSuffix(mem, "k")
	}

	var value int64
	if _, err := fmt.Sscanf(mem, "%d", &value); err != nil {
		return 0, fmt.Errorf("invalid memory value: %s", mem)
	}

	return value * multiplier, nil
}

// ParseCPUs converts a CPU string to a float.
func ParseCPUs(cpus string) (float64, error) {
	cpus = strings.TrimSpace(cpus)
	if cpus == "" {
		return 0, nil
	}

	var value float64
	if _, err := fmt.Sscanf(cpus, "%f", &value); err != nil {
		return 0, fmt.Errorf("invalid CPU value: %s", cpus)
	}

	return value, nil
}

// dangerousPaths are system directories that should never be mounted.
var dangerousPaths = []string{
	"/etc", "/root", "/sys", "/proc", "/dev", "/boot",
	"/var/run", "/var/log", "/usr", "/bin", "/sbin", "/lib",
}

// ValidateProjectPath checks that the path is safe to mount.
func ValidateProjectPath(projectPath string) error {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("resolving project path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("project path does not exist: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("project path is not a directory: %s", absPath)
	}

	for _, dangerous := range dangerousPaths {
		if absPath == dangerous || strings.HasPrefix(absPath, dangerous+"/") {
			return fmt.Errorf("refusing to mount system directory: %s", absPath)
		}
	}

	return nil
}

// ConfigToContainerConfig converts an agentbox config to container config.
func ConfigToContainerConfig(cfg *config.Config, projectPath string, cmd []string, env []string) (*ContainerConfig, error) {
	memory, err := ParseMemory(cfg.Docker.Resources.Memory)
	if err != nil {
		return nil, err
	}

	cpus, err := ParseCPUs(cfg.Docker.Resources.CPUs)
	if err != nil {
		return nil, err
	}

	if err := ValidateProjectPath(projectPath); err != nil {
		return nil, err
	}

	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return nil, fmt.Errorf("resolving project path: %w", err)
	}

	return &ContainerConfig{
		Name:        fmt.Sprintf("agentbox-%s", cfg.Project.Name),
		Image:       ImageName(cfg.Docker.Image),
		WorkDir:     "/workspace",
		ProjectPath: absPath,
		Env:         env,
		Cmd:         cmd,
		Network:     cfg.Docker.Network,
		Memory:      memory,
		CPUs:        cpus,
		MountSSH:    true,
		MountGit:    true,
	}, nil
}
