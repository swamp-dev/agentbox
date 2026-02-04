# Agentbox

Docker-sandboxed AI coding agent CLI with Ralph loop support.

## Features

- **Isolated Execution**: Run AI coding agents (Claude Code, Amp, Aider) in Docker containers
- **Ralph Pattern**: Iterative task completion with persistent state across iterations
- **Multi-Agent Support**: Plugin architecture for different AI agents
- **Security First**: Network isolation by default, controlled API access

## Prerequisites

- **Docker** — must be installed and running
- **Go 1.24.0+** — required to build from source
- **API keys** — see [Environment Variables](#environment-variables) for your agent

## Installation

```bash
# From source
go install github.com/swamp-dev/agentbox/cmd/agentbox@latest

# Or build locally
git clone https://github.com/swamp-dev/agentbox.git
cd agentbox
make build
```

## Quick Start

```bash
# Initialize a project
agentbox init

# Run a single agent session
agentbox run --agent claude --prompt "Fix the bug in auth.ts"

# Run Ralph loop until PRD complete
agentbox ralph --max-iterations 10 --prd prd.json

# Check progress
agentbox status
```

## Commands

| Command | Description |
|---------|-------------|
| `run` | Single agent session in container |
| `ralph` | Run Ralph loop until PRD complete |
| `init` | Initialize project with templates |
| `status` | Show Ralph loop progress |
| `images` | Manage base Docker images |
| `version` | Print version information |

See [docs/cli-reference.md](docs/cli-reference.md) for complete flag reference.

## Configuration

Create `agentbox.yaml` in your project root:

```yaml
version: "1.0"
project:
  name: "my-project"

agent:
  name: claude  # claude, claude-cli, amp, aider

docker:
  image: full   # node, python, go, rust, full
  resources:
    memory: "4g"
    cpus: "2"
  network: none  # isolated by default

ralph:
  max_iterations: 10
  prd_file: prd.json
  progress_file: progress.txt
  auto_commit: true
  quality_checks:
    - name: typecheck
      command: npm run typecheck
    - name: test
      command: npm test
  stop_signal: "<promise>COMPLETE</promise>"
```

## Ralph Pattern

> See [docs/prd-guide.md](docs/prd-guide.md) for the PRD schema reference and guide for writing effective PRDs.

The Ralph pattern enables iterative AI agent execution with memory persistence:

1. Spawn fresh container with agent
2. Load PRD, find next incomplete task
3. Run agent with task-specific prompt
4. Check for completion signal
5. Run quality checks
6. Commit changes to git
7. Update prd.json
8. Append learnings to progress.txt
9. Repeat until complete

**State persists via:**
- Git history (code changes)
- prd.json (task status)
- progress.txt (learnings)
- AGENTS.md (patterns discovered)

## Docker Images

| Image | Contents |
|-------|----------|
| `agentbox/node:20` | Node.js 20, npm, pnpm, Claude Code |
| `agentbox/python:3.12` | Python 3.12, pip, poetry, uv |
| `agentbox/go:1.22` | Go 1.22, common tools |
| `agentbox/rust:1.77` | Rust, cargo |
| `agentbox/full:latest` | All languages + all agents |

Build images locally:

```bash
make docker-build
```

## Security

**Isolated by default:**
- Filesystem: Only mounted `/workspace` accessible
- Network: No outbound (opt-in with `--allow-network`)
- Processes: Container PID namespace
- Docker: No access to host docker.sock

**Shared (read-only):**
- SSH keys (~/.ssh)
- Git config (~/.gitconfig)
- API keys (via environment)

## Environment Variables

| Variable | Agent | Required |
|----------|-------|----------|
| `ANTHROPIC_API_KEY` | claude, aider | Yes for claude |
| `OPENAI_API_KEY` | aider | Alternative for aider |
| `AMP_API_KEY` | amp | Yes for amp |
| *(none)* | claude-cli | Uses Claude subscription auth (`~/.claude/`). Run `claude login` first. |

## Development

```bash
# Run tests
make test

# Build binary
make build

# Format code
make fmt

# Run linters
make lint
```

## Documentation

- [CLI Reference](docs/cli-reference.md) — Complete flag reference for all commands
- [PRD Guide](docs/prd-guide.md) — PRD schema and guide for writing effective task definitions
- [Troubleshooting](docs/troubleshooting.md) — Solutions for common issues
- [Contributing](CONTRIBUTING.md) — Development setup and contributor guide
- [Security](SECURITY.md) — Security model and policies

## License

MIT
