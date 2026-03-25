# MCP Server

Agentbox includes a built-in [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) server that exposes all agentbox capabilities to any MCP-compatible client via stdio transport.

## Quick Start

```bash
agentbox mcp serve
```

This starts the MCP server on stdin/stdout using JSON-RPC 2.0.

## Client Configuration

### Claude Desktop

Add to your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "agentbox": {
      "command": "agentbox",
      "args": ["mcp", "serve"]
    }
  }
}
```

### VS Code (Copilot MCP)

Add to your VS Code settings:

```json
{
  "mcp.servers": {
    "agentbox": {
      "command": "agentbox",
      "args": ["mcp", "serve"]
    }
  }
}
```

## Available Tools

### `agentbox_run`

Run a single agent task in a sandboxed Docker container.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project_dir` | string | yes | Path to the project directory |
| `agent` | string | yes | Agent to use (claude, claude-cli, amp, aider) |
| `prompt` | string | yes | Prompt to send to the agent |
| `image` | string | no | Docker image type (node, python, go, rust, full) |
| `network` | string | no | Network mode (none, bridge, host) |

### `agentbox_ralph_start`

Start a Ralph loop for a PRD. Returns a session ID immediately (async).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project_dir` | string | yes | Path to the project directory |
| `agent` | string | no | Agent to use (default: claude) |
| `prd_file` | string | no | PRD file name (default: prd.json) |
| `max_iterations` | integer | no | Max iterations (default: 10) |

### `agentbox_sprint_start`

Start an autonomous sprint. Returns a session ID immediately (async).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project_dir` | string | no | Path to the project directory |
| `repo_url` | string | no | Git repository URL to clone |
| `prd_file` | string | no | PRD file name (default: prd.json) |
| `agent` | string | no | Agent to use (default: claude) |
| `sprint_size` | integer | no | Tasks per sprint (default: 5) |
| `max_sprints` | integer | no | Max sprints (default: 20) |

### `agentbox_status`

Check session/task progress.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `session_id` | string | no | Session ID to check |
| `project_dir` | string | no | Project directory for store lookup |

### `agentbox_journal`

Read dev diary entries for a session.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `session_id` | integer | yes | Session ID |
| `limit` | integer | no | Max entries to return |

### `agentbox_task_list`

List tasks from a PRD with status.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prd_file` | string | no | PRD file name (default: prd.json) |
| `project_dir` | string | no | Project directory (default: .) |

### `agentbox_sprint_status`

Monitor an active sprint.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `session_id` | string | yes | Session ID of the sprint |

## Protocol Details

The MCP server implements JSON-RPC 2.0 over stdio with the following methods:

- `initialize` -- Handshake returning server info and capabilities
- `tools/list` -- Returns all available tool definitions with JSON Schema input schemas
- `tools/call` -- Executes a tool and returns results
- `ping` -- Health check

No external MCP libraries are used; the protocol is implemented directly using the standard library.
