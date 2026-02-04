# Contributing to Agentbox

## Prerequisites

- **Go 1.24.0+** (module requires `go 1.24.0`)
- **Docker** (running and accessible)
- **make** (GNU Make)
- **golangci-lint** (for `make lint`)
- **goimports** (for `make fmt`)

## Development Setup

```bash
# Clone the repository
git clone https://github.com/swamp-dev/agentbox.git
cd agentbox

# Build the binary
make build
# Output: bin/agentbox

# Run tests
make test

# Run linters
make lint
```

## Project Structure

```
cmd/
  agentbox/         # CLI entrypoint (main.go)
internal/
  cli/              # Command definitions (cobra commands)
    run.go          # agentbox run
    ralph.go        # agentbox ralph
    init.go         # agentbox init
    status.go       # agentbox status
    images.go       # agentbox images (list/pull/build)
    version.go      # agentbox version
  config/           # Configuration loading and validation (agentbox.yaml)
  agent/            # Agent interface and implementations (claude, claude-cli, amp, aider)
  container/        # Docker container lifecycle management
  ralph/            # Ralph loop orchestration, PRD parsing, quality checks
```

## Adding a New Agent

1. **Implement the `Agent` interface** in `internal/agent/`:

```go
// internal/agent/agent.go
type Agent interface {
    Name() string
    Command(prompt string) []string
    Environment() []string
    StopSignal() string
    ParseOutput(output string) *AgentOutput
}
```

2. **Create your agent file** (e.g., `internal/agent/myagent.go`):

```go
type MyAgent struct{}

func NewMyAgent() *MyAgent { return &MyAgent{} }

func (a *MyAgent) Name() string                          { return "myagent" }
func (a *MyAgent) Command(prompt string) []string         { /* ... */ }
func (a *MyAgent) Environment() []string                  { /* ... */ }
func (a *MyAgent) StopSignal() string                     { /* ... */ }
func (a *MyAgent) ParseOutput(output string) *AgentOutput { /* ... */ }
```

3. **Register in `New()`** (`internal/agent/agent.go`):

```go
func New(name string) (Agent, error) {
    switch strings.ToLower(name) {
    case "claude":
        return NewClaudeAgent(), nil
    case "amp":
        return NewAmpAgent(), nil
    case "aider":
        return NewAiderAgent(), nil
    case "myagent":               // Add your agent
        return NewMyAgent(), nil
    default:
        return nil, fmt.Errorf("unknown agent: %s", name)
    }
}
```

4. **Add to config validation** in `internal/config/config.go` — add `"myagent"` to the valid agents list in `Validate()`.

5. **Add API key mapping** in `GetAPIKey()` if your agent requires one.

## Running Tests

```bash
# Run all tests
make test

# Run tests with coverage report
make test-coverage
# Opens coverage.html in browser

# Run linters
make lint

# Format code
make fmt
```

## Available Make Targets

| Target | Description |
|--------|-------------|
| `build` | Build binary to `bin/agentbox` |
| `install` | Install to `$GOPATH/bin` |
| `test` | Run `go test -v ./...` |
| `test-coverage` | Run tests with HTML coverage report |
| `lint` | Run `golangci-lint` |
| `fmt` | Format with `go fmt` + `goimports` |
| `clean` | Remove build artifacts |
| `docker-build` | Build all Docker images |
| `docker-build-full` | Build only the full image |
| `release` | Cross-compile for darwin/linux/windows |

## Commit & PR Conventions

### Conventional Commits

All commits must follow the [Conventional Commits](https://www.conventionalcommits.org/) format:

```
type(scope): description
```

| Type | Use for |
|------|---------|
| `feat` | New functionality |
| `fix` | Bug fixes |
| `test` | Adding or updating tests |
| `refactor` | Code changes without behavior change |
| `docs` | Documentation only |
| `chore` | Build, config, dependencies |

Examples:

```bash
git commit -m "feat(agent): add support for Copilot agent"
git commit -m "fix(ralph): handle empty PRD gracefully"
git commit -m "test(container): add network isolation tests"
```

### Pull Requests

- Create a feature branch from `main`
- Keep PRs focused on a single change
- Ensure `make test` and `make lint` pass
- Use squash merge when merging to `main`
