package container

import "testing"

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
