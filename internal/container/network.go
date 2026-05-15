package container

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
)

// RestrictedNetwork holds the resources for an egress-restricted network setup.
type RestrictedNetwork struct {
	NetworkID   string
	NetworkName string
	ProxyID     string
	ProxyName   string
	ProxyIP     string // IP on the internal network
}

// ProxyContainerName returns the deterministic name for the proxy sidecar.
func ProxyContainerName(baseName string) string {
	return fmt.Sprintf("agentbox-proxy-%s", baseName)
}

// RestrictedNetworkName returns the deterministic name for the internal network.
func RestrictedNetworkName(baseName string) string {
	return fmt.Sprintf("agentbox-net-%s", baseName)
}

// CreateRestrictedNetwork creates a Docker internal network and a proxy sidecar
// container that enforces egress restrictions. The proxy container is created on
// Docker's default bridge (for internet access) and then connected to the internal
// network (for agent communication). The agent container should only be on the
// internal network.
func (m *Manager) CreateRestrictedNetwork(ctx context.Context, baseName string, agentImage string, allowedHosts []string) (*RestrictedNetwork, error) {
	netName := RestrictedNetworkName(baseName)
	proxyName := ProxyContainerName(baseName)

	// Remove any stale network and proxy container from a prior run that
	// wasn't cleaned up (e.g., crash, kill -9). Without this, Docker returns
	// "network already exists" and blocks subsequent runs.
	m.removeStaleNetwork(ctx, netName, proxyName)

	// Create internal network — no default gateway to internet.
	netResp, err := m.client.NetworkCreate(ctx, netName, network.CreateOptions{
		Driver:   "bridge",
		Internal: true,
		Labels: map[string]string{
			"managed-by": "agentbox",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating restricted network: %w", err)
	}

	rn := &RestrictedNetwork{
		NetworkID:   netResp.ID,
		NetworkName: netName,
		ProxyName:   proxyName,
	}

	// Build the proxy command.
	proxyCmd := []string{"/usr/local/bin/agentbox", "proxy", "--addr", "0.0.0.0:3128"}
	for _, h := range allowedHosts {
		proxyCmd = append(proxyCmd, "--allow", h)
	}

	// Find the agentbox binary on the host to bind-mount into the proxy container.
	agentboxBin, err := os.Executable()
	if err != nil {
		_ = m.RemoveRestrictedNetwork(ctx, rn)
		return nil, fmt.Errorf("finding agentbox binary: %w", err)
	}

	// Create proxy container on Docker's default bridge network (by omitting
	// NetworkMode, Docker uses the default bridge). This avoids hardcoding the
	// network name "bridge" which may not exist on all Docker installations.
	proxyResp, err := m.client.ContainerCreate(ctx,
		&dockercontainer.Config{
			Image: agentImage,
			Cmd:   proxyCmd,
			Labels: map[string]string{
				"managed-by": "agentbox",
			},
		},
		&dockercontainer.HostConfig{
			Mounts: []mount.Mount{
				{
					Type:     mount.TypeBind,
					Source:   agentboxBin,
					Target:   "/usr/local/bin/agentbox",
					ReadOnly: true,
				},
			},
		},
		nil, nil, proxyName,
	)
	if err != nil {
		_ = m.RemoveRestrictedNetwork(ctx, rn)
		return nil, fmt.Errorf("creating proxy container: %w", err)
	}
	rn.ProxyID = proxyResp.ID

	// Connect the proxy to the internal network so the agent container can reach it.
	// Docker embedded DNS resolves container names on user-defined networks, so the
	// agent container can reach the proxy by name (e.g., "agentbox-proxy-myproject").
	if err := m.client.NetworkConnect(ctx, rn.NetworkID, rn.ProxyID, nil); err != nil {
		_ = m.RemoveRestrictedNetwork(ctx, rn)
		return nil, fmt.Errorf("connecting proxy to restricted network: %w", err)
	}

	// Start the proxy container.
	if err := m.client.ContainerStart(ctx, rn.ProxyID, dockercontainer.StartOptions{}); err != nil {
		_ = m.RemoveRestrictedNetwork(ctx, rn)
		return nil, fmt.Errorf("starting proxy container: %w", err)
	}

	// Discover the proxy's IP on the internal network so we can inject it
	// into the agent container's /etc/hosts via ExtraHosts.
	inspect, err := m.client.ContainerInspect(ctx, rn.ProxyID)
	if err != nil {
		_ = m.RemoveRestrictedNetwork(ctx, rn)
		return nil, fmt.Errorf("inspecting proxy container: %w", err)
	}
	netInfo, ok := inspect.NetworkSettings.Networks[rn.NetworkName]
	if !ok || netInfo.IPAddress == "" {
		_ = m.RemoveRestrictedNetwork(ctx, rn)
		return nil, fmt.Errorf("proxy container has no IP on network %s", rn.NetworkName)
	}
	rn.ProxyIP = netInfo.IPAddress

	return rn, nil
}

// removeStaleNetwork removes a leftover network and proxy container from a prior
// run. This is best-effort — errors are silently ignored since the resources may
// not exist.
func (m *Manager) removeStaleNetwork(ctx context.Context, netName, proxyName string) {
	timeout := 5
	_ = m.client.ContainerStop(ctx, proxyName, dockercontainer.StopOptions{Timeout: &timeout})
	_ = m.client.ContainerRemove(ctx, proxyName, dockercontainer.RemoveOptions{Force: true})
	_ = m.client.NetworkRemove(ctx, netName)
}

// resolveExtraHosts resolves allowed endpoint hostnames on the host and returns
// Docker ExtraHosts entries (hostname:ip). This allows containers on internal
// networks (which lack external DNS) to reach allowed destinations.
// The proxyName and proxyIP are also added so the agent can reach the proxy.
func resolveExtraHosts(allowedHosts []string, proxyName, proxyIP string) ([]string, error) {
	seen := make(map[string]bool)
	var hosts []string

	for _, hostPort := range allowedHosts {
		host, _, err := net.SplitHostPort(hostPort)
		if err != nil {
			// No port component — treat the whole string as a host.
			host = hostPort
		}

		if seen[host] {
			continue
		}
		seen[host] = true

		// Skip if it's already an IP address.
		if net.ParseIP(host) != nil {
			continue
		}

		ips, err := net.LookupHost(host)
		if err != nil {
			return nil, fmt.Errorf("resolving %s: %w", host, err)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("no addresses found for %s", host)
		}
		hosts = append(hosts, fmt.Sprintf("%s:%s", host, ips[0]))
	}

	// Add proxy container mapping.
	if proxyName != "" && proxyIP != "" {
		hosts = append(hosts, fmt.Sprintf("%s:%s", proxyName, proxyIP))
	}

	return hosts, nil
}

// RemoveRestrictedNetwork tears down the proxy container and internal network.
// It attempts all cleanup steps even if individual steps fail.
func (m *Manager) RemoveRestrictedNetwork(ctx context.Context, rn *RestrictedNetwork) error {
	var errs []error

	if rn.ProxyID != "" {
		timeout := 5
		_ = m.client.ContainerStop(ctx, rn.ProxyID, dockercontainer.StopOptions{Timeout: &timeout})
		if err := m.client.ContainerRemove(ctx, rn.ProxyID, dockercontainer.RemoveOptions{Force: true}); err != nil {
			errs = append(errs, fmt.Errorf("removing proxy container: %w", err))
		}
	}

	if rn.NetworkID != "" {
		if err := m.client.NetworkRemove(ctx, rn.NetworkID); err != nil {
			errs = append(errs, fmt.Errorf("removing restricted network: %w", err))
		}
	}

	return errors.Join(errs...)
}
