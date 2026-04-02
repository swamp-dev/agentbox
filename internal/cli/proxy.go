package cli

import (
	"github.com/spf13/cobra"

	"github.com/swamp-dev/agentbox/internal/proxy"
)

var (
	proxyAllow []string
	proxyAddr  string
)

var proxyCmd = &cobra.Command{
	Use:    "proxy",
	Short:  "Run egress-filtering HTTP proxy (internal use)",
	Hidden: true,
	RunE:   runProxy,
}

func init() {
	proxyCmd.Flags().StringSliceVar(&proxyAllow, "allow", nil, "allowed host:port endpoints")
	proxyCmd.Flags().StringVar(&proxyAddr, "addr", "0.0.0.0:3128", "listen address")
}

func runProxy(cmd *cobra.Command, args []string) error {
	hosts := make(map[string]bool, len(proxyAllow))
	for _, h := range proxyAllow {
		hosts[h] = true
	}

	logger.Info("starting egress proxy", "addr", proxyAddr, "allowed", proxyAllow)

	p := &proxy.EgressProxy{AllowedHosts: hosts, Addr: proxyAddr}
	return p.ListenAndServe()
}
