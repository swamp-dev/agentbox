// Package container provides Docker container management for agentbox.
package container

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"github.com/swamp-dev/agentbox/internal/config"
)

// Manager handles Docker container lifecycle.
type Manager struct {
	client         *client.Client
	restrictedNets map[string]*RestrictedNetwork // containerID -> restricted network
}

// NewManager creates a new Docker container manager.
func NewManager() (*Manager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("creating docker client: %w", err)
	}

	return &Manager{
		client:         cli,
		restrictedNets: make(map[string]*RestrictedNetwork),
	}, nil
}

// Close releases the Docker client resources.
func (m *Manager) Close() error {
	return m.client.Close()
}

// ContainerConfig holds all settings for creating a container.
type ContainerConfig struct {
	Name              string
	Image             string
	WorkDir           string
	ProjectPath       string
	Env               []string
	Cmd               []string
	Network           string
	AllowedEndpoints  []string // host:port pairs for restricted network mode
	Memory            int64
	CPUs              float64
	MountSSH          bool
	MountGit          bool
	MountClaudeConfig bool
	Interactive       bool // allocate TTY and keep stdin open
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

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory for mounts: %w", err)
	}

	if cfg.MountSSH {
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

	// NOTE: claude-cli credentials (~/.claude/.credentials.json) are injected
	// via CopyToContainer after ContainerCreate — not as a bind mount — so the
	// file is readable by the container's agent user regardless of host
	// permissions. We do NOT mount ~/.claude.json because Claude Code tries to
	// write to it during execution and a read-only mount causes it to hang.

	containerCfg := &container.Config{
		Image:      cfg.Image,
		Cmd:        cfg.Cmd,
		Env:        cfg.Env,
		WorkingDir: "/workspace",
		Tty:        cfg.Interactive,
		OpenStdin:  cfg.Interactive,
	}

	hostCfg := &container.HostConfig{
		Mounts: mounts,
		Resources: container.Resources{
			Memory:   cfg.Memory,
			NanoCPUs: int64(cfg.CPUs * 1e9),
		},
	}

	networkCfg := &network.NetworkingConfig{}

	var rn *RestrictedNetwork
	switch cfg.Network {
	case "none":
		hostCfg.NetworkMode = "none"
	case "host":
		hostCfg.NetworkMode = "host"
	case "restricted":
		var err error
		rn, err = m.CreateRestrictedNetwork(ctx, cfg.Name, cfg.Image, cfg.AllowedEndpoints)
		if err != nil {
			return "", fmt.Errorf("setting up restricted network: %w", err)
		}
		// Place agent container on the internal network only.
		hostCfg.NetworkMode = container.NetworkMode(rn.NetworkName)
		// Inject proxy env vars so tools use the proxy.
		proxyURL := fmt.Sprintf("http://%s:3128", rn.ProxyName)
		containerCfg.Env = append(containerCfg.Env,
			"HTTP_PROXY="+proxyURL,
			"HTTPS_PROXY="+proxyURL,
			"http_proxy="+proxyURL,
			"https_proxy="+proxyURL,
		)
	}

	resp, err := m.client.ContainerCreate(ctx, containerCfg, hostCfg, networkCfg, nil, cfg.Name)
	if err != nil {
		if rn != nil {
			_ = m.RemoveRestrictedNetwork(ctx, rn)
		}
		return "", fmt.Errorf("creating container: %w", err)
	}

	// Inject claude-cli credentials into the container before starting.
	// We copy instead of bind-mounting because the host file may be owned
	// by root with 0600 permissions, making it unreadable by the container's
	// agent user (UID 1000).
	if cfg.MountClaudeConfig {
		credFile := filepath.Join(home, ".claude", ".credentials.json")
		if data, err := os.ReadFile(credFile); err == nil {
			if copyErr := m.copyFileToContainer(ctx, resp.ID, "/home/agent/.claude/.credentials.json", data, 1000, 1000); copyErr != nil {
				_ = m.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
				if rn != nil {
					_ = m.RemoveRestrictedNetwork(ctx, rn)
				}
				return "", fmt.Errorf("injecting credentials: %w", copyErr)
			}
		}
	}

	if err := m.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		_ = m.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		if rn != nil {
			_ = m.RemoveRestrictedNetwork(ctx, rn)
		}
		return "", fmt.Errorf("starting container: %w", err)
	}

	if rn != nil {
		m.restrictedNets[resp.ID] = rn
	}

	return resp.ID, nil
}

// Run creates a container, runs the command, and returns the output.
func (m *Manager) Run(ctx context.Context, cfg *ContainerConfig) (string, error) {
	containerID, err := m.Create(ctx, cfg)
	if err != nil {
		return "", err
	}
	defer func() {
		// Use a fresh context for cleanup — the original ctx may be cancelled.
		rmCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = m.Remove(rmCtx, containerID)
	}()

	return m.Wait(ctx, containerID)
}

// Wait blocks until the container exits and returns its output.
// It respects context cancellation — if the context is cancelled or times out,
// the container is killed and partial logs are returned.
func (m *Manager) Wait(ctx context.Context, containerID string) (string, error) {
	statusCh, errCh := m.client.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)

	select {
	case <-ctx.Done():
		return m.killAndCollectLogs(containerID, ctx.Err())
	case err := <-errCh:
		if err != nil {
			// ContainerWait can surface context cancellation via errCh rather
			// than ctx.Done() — check ctx.Err() to ensure we still clean up.
			if ctx.Err() != nil {
				return m.killAndCollectLogs(containerID, ctx.Err())
			}
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

// killAndCollectLogs stops a container and returns whatever logs are available.
// Used when a context timeout or cancellation requires forceful cleanup.
func (m *Manager) killAndCollectLogs(containerID string, cause error) (string, error) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = m.Stop(cleanupCtx, containerID)
	logs, _ := m.Logs(cleanupCtx, containerID)
	return logs, fmt.Errorf("waiting for container: %w", cause)
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

	// When TTY is enabled, Docker streams raw output (no multiplexing).
	// stdcopy.StdCopy only works with non-TTY multiplexed streams.
	inspect, inspectErr := m.client.ContainerInspect(ctx, containerID)
	if inspectErr == nil && inspect.Config != nil && inspect.Config.Tty {
		raw, readErr := io.ReadAll(out)
		if readErr != nil {
			return "", fmt.Errorf("reading container logs: %w", readErr)
		}
		return stripANSI(string(raw)), nil
	}

	var stdout, stderr strings.Builder
	if _, err := stdcopy.StdCopy(&stdout, &stderr, out); err != nil {
		return "", fmt.Errorf("reading container logs: %w", err)
	}

	return stdout.String() + stderr.String(), nil
}

// stripANSI removes ANSI escape codes from a string.
func stripANSI(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			// Skip ESC [ ... <final byte> sequences
			if i+1 < len(s) && s[i+1] == '[' {
				j := i + 2
				for j < len(s) && ((s[j] >= '0' && s[j] <= '9') || s[j] == ';' || s[j] == '?') {
					j++
				}
				if j < len(s) {
					j++ // skip final byte
				}
				i = j
				continue
			}
			// Skip other ESC sequences (ESC + one byte)
			i += 2
			continue
		}
		result.WriteByte(s[i])
		i++
	}
	return result.String()
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

// copyFileToContainer writes a single file into a container using a tar archive.
// The file is created with the specified UID/GID ownership and 0600 permissions.
func (m *Manager) copyFileToContainer(ctx context.Context, containerID, destPath string, content []byte, uid, gid int) error {
	dir := filepath.Dir(destPath)
	name := filepath.Base(destPath)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Size: int64(len(content)),
		Mode: 0o600,
		Uid:  uid,
		Gid:  gid,
	}); err != nil {
		return err
	}
	if _, err := tw.Write(content); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}

	return m.client.CopyToContainer(ctx, containerID, dir, &buf, container.CopyToContainerOptions{})
}

// Stop gracefully stops a running container.
func (m *Manager) Stop(ctx context.Context, containerID string) error {
	timeout := 10
	return m.client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

// Remove deletes a container and its associated restricted network, if any.
func (m *Manager) Remove(ctx context.Context, containerID string) error {
	err := m.client.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})

	if rn, ok := m.restrictedNets[containerID]; ok {
		delete(m.restrictedNets, containerID)
		if rnErr := m.RemoveRestrictedNetwork(ctx, rn); rnErr != nil && err == nil {
			err = rnErr
		}
	}

	return err
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

// systemPaths blocks the path and all subdirectories from being mounted.
var systemPaths = []string{
	"/etc", "/sys", "/proc", "/dev", "/boot",
	"/var/run", "/var/log", "/usr", "/bin", "/sbin", "/lib",
}

// exactBlockPaths blocks only the exact path (subdirectories are allowed).
// /root itself is dangerous to mount, but /root/projects is fine.
var exactBlockPaths = []string{"/root"}

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

	for _, blocked := range systemPaths {
		if absPath == blocked || strings.HasPrefix(absPath, blocked+"/") {
			return fmt.Errorf("refusing to mount system directory: %s", absPath)
		}
	}

	for _, blocked := range exactBlockPaths {
		if absPath == blocked {
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
		Name:              fmt.Sprintf("agentbox-%s", cfg.Project.Name),
		Image:             ImageName(cfg.Docker.Image),
		WorkDir:           "/workspace",
		ProjectPath:       absPath,
		Env:               env,
		Cmd:               cmd,
		Network:           cfg.Docker.Network,
		AllowedEndpoints:  cfg.Docker.AllowedEndpoints,
		Memory:            memory,
		CPUs:              cpus,
		MountSSH:          true,
		MountGit:          true,
		MountClaudeConfig: cfg.Agent.Name == "claude-cli",
	}, nil
}
