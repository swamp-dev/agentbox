# CLI Reference

Complete flag reference for all agentbox commands.

## `agentbox run`

Run a single agent session in a Docker container.

```
agentbox run [flags]
```

### Flags

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--agent` | `-a` | `string` | `claude` | Agent to use (`claude`, `claude-cli`, `amp`, `aider`) |
| `--project` | `-p` | `string` | `.` | Project directory to mount into the container |
| `--prompt` | | `string` | | Prompt to send to the agent |
| `--network` | | `string` | `none` | Network mode (`none`, `bridge`, `host`) |
| `--image` | | `string` | `full` | Docker image to use (`node`, `python`, `go`, `rust`, `full`) |
| `--interactive` | `-i` | `bool` | `false` | Run in interactive mode |
| `--allow-network` | | `bool` | `false` | Allow outbound network access (overrides `--network` to `bridge`) |

### Examples

```bash
# Run Claude with a prompt
agentbox run --agent claude --prompt "Fix the login bug"

# Use a specific image with network access
agentbox run --agent aider --image python --allow-network --prompt "Add API client"

# Interactive session
agentbox run -a claude -i

# Mount a different project directory
agentbox run -p /path/to/project --prompt "Run tests and fix failures"
```

---

## `agentbox ralph`

Run the Ralph loop — iteratively executes agent sessions until all PRD tasks are complete.

```
agentbox ralph [flags]
```

### Flags

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--agent` | `-a` | `string` | `claude` | Agent to use (`claude`, `claude-cli`, `amp`, `aider`) |
| `--project` | `-p` | `string` | `.` | Project directory |
| `--max-iterations` | | `int` | `10` | Maximum iterations before stopping |
| `--prd` | | `string` | `prd.json` | Path to the PRD file |
| `--auto-commit` | | `bool` | `true` | Automatically commit changes after each task |

### Examples

```bash
# Run with defaults
agentbox ralph

# Use a custom PRD with more iterations
agentbox ralph --prd tasks/sprint-1.json --max-iterations 20

# Use Aider without auto-commit
agentbox ralph --agent aider --auto-commit=false

# Specify project directory
agentbox ralph -p /path/to/project --prd prd.json
```

---

## `agentbox init`

Initialize a project with agentbox configuration files.

```
agentbox init [flags]
```

### Flags

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--template` | `-t` | `string` | `standard` | Template to use (`standard`, `minimal`) |
| `--language` | `-l` | `string` | | Language/runtime (`node`, `python`, `go`, `rust`) |
| `--name` | `-n` | `string` | | Project name (defaults to current directory name) |
| `--force` | `-f` | `bool` | `false` | Overwrite existing files |

### Templates

- **`standard`** — Full setup with quality checks (`typecheck` and `test` commands pre-configured)
- **`minimal`** — Basic config with no quality checks

### Files Created

| File | Purpose |
|------|---------|
| `agentbox.yaml` | Project configuration |
| `prd.json` | Task definitions (default PRD) |
| `progress.txt` | Execution log / learnings |
| `AGENTS.md` | Agent patterns and conventions |

### Examples

```bash
# Initialize with defaults
agentbox init

# Minimal setup for a Go project
agentbox init --template minimal --language go --name my-service

# Overwrite existing config
agentbox init --force
```

---

## `agentbox status`

Show Ralph loop progress for the current project.

```
agentbox status [flags]
```

### Flags

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--project` | `-p` | `string` | `.` | Project directory |
| `--prd` | | `string` | `prd.json` | PRD file path |
| `--json` | | `bool` | `false` | Output in JSON format |

### JSON Output Format

When `--json` is used, output follows this structure:

```json
{
  "project": "my-project",
  "progress": 0.6,
  "tasks": {
    "total": 5,
    "completed": 3,
    "in_progress": 1,
    "pending": 1
  },
  "is_complete": false
}
```

### Examples

```bash
# Show progress for current directory
agentbox status

# JSON output for scripting
agentbox status --json

# Check a specific project
agentbox status -p /path/to/project --prd custom-prd.json
```

---

## `agentbox images`

Manage Docker images used by agentbox.

```
agentbox images <subcommand>
```

### Subcommands

#### `agentbox images list`

List all available agentbox images with their descriptions.

```bash
agentbox images list
```

**Available images:**

| Name | Tag | Contents |
|------|-----|----------|
| `agentbox/node` | `20` | Node.js 20, npm, pnpm, Claude Code |
| `agentbox/python` | `3.12` | Python 3.12, pip, poetry, uv |
| `agentbox/go` | `1.22` | Go 1.22, common tools |
| `agentbox/rust` | `1.77` | Rust, cargo |
| `agentbox/full` | `latest` | All languages + all agents |

#### `agentbox images pull [image]`

Pull agentbox images from the registry.

```bash
# Pull all images
agentbox images pull

# Pull a specific image
agentbox images pull node
```

#### `agentbox images build [image]`

Build agentbox images locally from Dockerfiles.

```bash
# Build all images
agentbox images build

# Build a specific image
agentbox images build full
```

---

## `agentbox version`

Print version information.

```bash
agentbox version
```

### Output

```
agentbox v0.1.0
  commit:     abc1234
  built:      2025-01-15T10:30:00Z
  go version: go1.24.0
  platform:   linux/amd64
```

Fields:
- **version** — Release tag (or `dev` for local builds)
- **commit** — Git commit hash (or `none` for local builds)
- **built** — Build timestamp (or `unknown` for local builds)
- **go version** — Go runtime version
- **platform** — OS and architecture
