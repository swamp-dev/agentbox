package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/swamp-dev/agentbox/internal/mcp"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP (Model Context Protocol) server commands",
	Long: `MCP server commands for exposing agentbox capabilities to MCP clients.

The MCP server uses JSON-RPC 2.0 over stdio to communicate with any
MCP-compatible client (Claude Desktop, VS Code, etc).`,
}

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MCP server on stdio",
	Long: `Start an MCP server that reads JSON-RPC requests from stdin and writes
responses to stdout. This is the standard MCP stdio transport.

Usage with Claude Desktop (claude_desktop_config.json):
  {
    "mcpServers": {
      "agentbox": {
        "command": "agentbox",
        "args": ["mcp", "serve"]
      }
    }
  }`,
	RunE: runMCPServe,
}

func init() {
	mcpCmd.AddCommand(mcpServeCmd)
}

func runMCPServe(cmd *cobra.Command, args []string) error {
	srv := mcp.NewServer(os.Stdin, os.Stdout)
	return srv.Run()
}
