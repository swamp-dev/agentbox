# Agentbox Development Guide

Docker-sandboxed AI coding agent CLI written in Go. This file defines how we work on this project.

## Quick Reference

```bash
make build          # Build binary to bin/agentbox
make test           # go test -v ./...
make test-coverage  # Coverage report → coverage.html
make lint           # golangci-lint run ./...
make fmt            # go fmt + goimports
```

CI runs: build → lint → test (with `-race`) → coverage threshold (40% floor, target 70-80%).

## Development Workflow

Follow the end-to-end process for every change. No exceptions.

### 1. Worktree + Branch

Always work in an isolated worktree off `main`:

```bash
git fetch origin main
git worktree add ../TICKET-XXX-description -b feat/TICKET-XXX-description origin/main
cd ../TICKET-XXX-description
```

Branch prefixes: `feat/`, `fix/`, `refactor/`, `test/`, `docs/`, `chore/`.

### 2. Research + Clarify

Read the relevant code before writing anything. Break the task into testable units. Surface ambiguities early. Don't start coding until you know what "done" looks like.

### 3. TDD Loop (Red → Green → Refactor)

Every unit of work follows this cycle:

```bash
# Write failing test → commit
go test -v -run TestMyNewThing ./internal/mypkg/
git commit -m "test(mypkg): add failing test for new thing"

# Make it pass → commit
go test -v -run TestMyNewThing ./internal/mypkg/
git commit -m "feat(mypkg): implement new thing"

# Refactor if needed → commit
go test -v ./internal/mypkg/
git commit -m "refactor(mypkg): extract helper from new thing"
```

Build up from unit tests to integration tests as units come together.

### 4. Quality Checks

All must pass before requesting review:

```bash
make fmt            # Format code (goimports + go fmt)
make lint           # No lint errors (includes format checks)
make test           # All tests pass
```

CI also runs with `-race` — if you suspect concurrency issues, test locally with `go test -race ./...`.

Do not skip or disable checks.

### 5. Code Review

Get review **before** opening a PR. Address critical/significant issues immediately. Minor issues can go in a follow-up.

### 6. Open PR

```bash
git push -u origin feat/TICKET-XXX-description
gh pr create --title "feat(scope): short description" --body "..."
```

### 7. Verify CI → Squash Merge → Clean Up

Wait for green CI. Squash merge to `main`. Remove the worktree:

```bash
git fetch origin
git worktree remove ../TICKET-XXX-description
git branch -d feat/TICKET-XXX-description
```

## Commit Conventions

Follow [Conventional Commits](https://www.conventionalcommits.org/) — see CONTRIBUTING.md for the full type table. Scope is the package name (e.g., `ralph`, `supervisor`, `agent`, `store`).

Small, atomic commits that alternate test → implementation:

```
test(ralph): add failing test for empty PRD handling
feat(ralph): handle empty PRD gracefully
test(ralph): add edge case for single-task PRD
```

## Go Conventions

### Project Structure

```
cmd/agentbox/           # CLI entrypoint
internal/
  agent/                # Agent interface + implementations
  cli/                  # Cobra commands (run, ralph, sprint, init, status, images, journal, dashboard)
  config/               # Config loading (agentbox.yaml)
  container/            # Docker lifecycle
  ralph/                # Ralph loop, PRD parsing, progress
  supervisor/           # Sprint orchestration, adaptive control
  store/                # SQLite persistence
  taskdb/               # In-memory task database
  workflow/             # Git operations
  journal/              # Session journaling
  metrics/              # Token usage, budgets
  retro/                # Sprint retrospectives
  review/               # Code review gate
```

### Test Patterns

Use **table-driven tests** — this is the established pattern across the project:

```go
func TestMyFunction(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"valid input", "foo", "bar", false},
        {"empty input", "", "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := MyFunction(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("MyFunction(%s) error = %v, wantErr %v", tt.input, err, tt.wantErr)
            }
            if got != tt.want {
                t.Errorf("MyFunction(%s) = %v, want %v", tt.input, got, tt.want)
            }
        })
    }
}
```

Test files live alongside their source: `mypkg/thing.go` → `mypkg/thing_test.go`.

Use `t.Helper()` in test helpers so failures report the caller's line. Use `t.TempDir()` for throwaway directories. For store tests, use in-memory SQLite:

```go
func openTestStore(t *testing.T) *store.Store {
    t.Helper()
    s, err := store.Open(":memory:")
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { s.Close() })
    return s
}
```

### Interfaces for Testability

Mock at boundaries using interfaces (see `store.Store`, `AgentRunner`, `NoopAgentRunner` for dry-run/testing). Don't mock internal functions.

### Error Handling

Always check errors. `golangci-lint` enforces `errcheck`. Wrap errors with context:

```go
if err != nil {
    return fmt.Errorf("loading PRD: %w", err)
}
```

### Naming

| Type | Convention | Example |
|------|------------|---------|
| Packages | lowercase, single word | `store`, `taskdb`, `retro` |
| Exported funcs | PascalCase | `NewCollector`, `RunSprint` |
| Unexported funcs | camelCase | `importPRD`, `runReviewGate` |
| Constants | PascalCase | `StatusPending`, `MaxRetries` |
| Test funcs | `TestXxx` | `TestNewAgent`, `TestValidateConfig` |

## Bug Fix Process

1. Write a failing test that reproduces the bug
2. Document why existing tests didn't catch it (in PR description)
3. Fix the bug (minimum change)
4. Verify the test passes, run full suite

## Architecture Notes

### Ralph vs Supervisor

- **Ralph** (`internal/ralph/`): Single-loop execution engine. Iterates over PRD tasks, runs agent in container, checks quality, commits, updates PRD.
- **Supervisor** (`internal/supervisor/`): Multi-sprint orchestrator built on top of Ralph. Adds worktrees, budgets, retrospectives, adaptive control, review gates, and PR creation.
- `RalphAgentRunner` adapts Ralph's `Loop` to the Supervisor's `AgentRunner` interface.

### Key Interfaces

- `agent.Agent` — pluggable agent implementations (Claude, Amp, Aider, Claude CLI). To add a new agent: implement the interface in `internal/agent/`, register in `New()`, add to config validation. See CONTRIBUTING.md for the full walkthrough.
- `supervisor.AgentRunner` — abstraction for running a single task iteration (`RalphAgentRunner` for real runs, `NoopAgentRunner` for dry-run/testing)
- `store.Store` — SQLite persistence for sessions, tasks, attempts, metrics

## What NOT to Do

- Don't commit directly to `main`
- Don't skip `make lint` — CI will catch it anyway
- Don't add `//nolint` without a justifying comment
- Don't mock the database in integration tests — use real SQLite
- Don't add dependencies without checking if the stdlib covers it
- Don't write tests that depend on execution order
