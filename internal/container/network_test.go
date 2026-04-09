package container

import (
	"fmt"
	"strings"
	"testing"
)

func TestStaleNetworkNamingConsistency(t *testing.T) {
	// removeStaleNetwork uses the same naming convention as CreateRestrictedNetwork.
	// Verify the names match so stale cleanup targets the right resources.
	baseName := "my-project"
	netName := RestrictedNetworkName(baseName)
	proxyName := ProxyContainerName(baseName)

	if netName != fmt.Sprintf("agentbox-net-%s", baseName) {
		t.Errorf("unexpected network name: %s", netName)
	}
	if proxyName != fmt.Sprintf("agentbox-proxy-%s", baseName) {
		t.Errorf("unexpected proxy name: %s", proxyName)
	}
}

func TestProxyContainerName(t *testing.T) {
	tests := []struct {
		name     string
		baseName string
		want     string
	}{
		{"simple name", "my-project", "agentbox-proxy-my-project"},
		{"empty name", "", "agentbox-proxy-"},
		{"with numbers", "project-123", "agentbox-proxy-project-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProxyContainerName(tt.baseName); got != tt.want {
				t.Errorf("ProxyContainerName(%q) = %q, want %q", tt.baseName, got, tt.want)
			}
		})
	}
}

func TestRestrictedNetworkName(t *testing.T) {
	tests := []struct {
		name     string
		baseName string
		want     string
	}{
		{"simple name", "my-project", "agentbox-net-my-project"},
		{"empty name", "", "agentbox-net-"},
		{"with numbers", "project-123", "agentbox-net-project-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RestrictedNetworkName(tt.baseName); got != tt.want {
				t.Errorf("RestrictedNetworkName(%q) = %q, want %q", tt.baseName, got, tt.want)
			}
		})
	}
}

func TestResolveExtraHosts(t *testing.T) {
	tests := []struct {
		name         string
		allowedHosts []string
		proxyName    string
		proxyIP      string
		wantProxy    bool
		wantErr      bool
	}{
		{
			name:         "resolves known host",
			allowedHosts: []string{"api.anthropic.com:443"},
			proxyName:    "proxy",
			proxyIP:      "172.18.0.2",
			wantProxy:    true,
		},
		{
			name:         "deduplicates same host different ports",
			allowedHosts: []string{"api.anthropic.com:443", "api.anthropic.com:80"},
			proxyName:    "proxy",
			proxyIP:      "172.18.0.2",
			wantProxy:    true,
		},
		{
			name:         "skips IP addresses",
			allowedHosts: []string{"192.168.1.1:443"},
			proxyName:    "proxy",
			proxyIP:      "172.18.0.2",
			wantProxy:    true,
		},
		{
			name:         "unresolvable host",
			allowedHosts: []string{"this-host-does-not-exist-xyz.invalid:443"},
			wantErr:      true,
		},
		{
			name:         "empty proxy info",
			allowedHosts: []string{"api.anthropic.com:443"},
			proxyName:    "",
			proxyIP:      "",
			wantProxy:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hosts, err := resolveExtraHosts(tt.allowedHosts, tt.proxyName, tt.proxyIP)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveExtraHosts() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if tt.wantProxy {
				want := tt.proxyName + ":" + tt.proxyIP
				found := false
				for _, h := range hosts {
					if h == want {
						found = true
					}
				}
				if !found {
					t.Errorf("missing proxy entry %q in %v", want, hosts)
				}
			}
		})
	}
}

func TestResolveExtraHosts_Dedup(t *testing.T) {
	hosts, err := resolveExtraHosts(
		[]string{"api.anthropic.com:443", "api.anthropic.com:80"},
		"", "",
	)
	if err != nil {
		t.Fatal(err)
	}

	// Should have exactly 1 entry (deduplicated), not 2.
	count := 0
	for _, h := range hosts {
		if strings.HasPrefix(h, "api.anthropic.com:") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 entry for api.anthropic.com, got %d in %v", count, hosts)
	}
}
