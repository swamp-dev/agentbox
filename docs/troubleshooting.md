# Troubleshooting

## Docker Issues

### Docker not running

```
Error: Cannot connect to the Docker daemon
```

Start the Docker daemon:

```bash
# Linux
sudo systemctl start docker

# macOS
open -a Docker
```

### Permission denied

```
Error: permission denied while trying to connect to the Docker daemon socket
```

Add your user to the `docker` group:

```bash
sudo usermod -aG docker $USER
# Log out and back in for the change to take effect
```

### Image not found

```
Error: image "agentbox/full:latest" not found
```

Pull or build the image:

```bash
# Pull from registry
agentbox images pull full

# Or build locally
agentbox images build full

# Or build all images
make docker-build
```

---

## API Key Issues

Each agent requires specific API keys passed via environment variables:

| Agent | Required Variable | Fallback |
|-------|-------------------|----------|
| `claude` | `ANTHROPIC_API_KEY` | — |
| `amp` | `AMP_API_KEY` | — |
| `aider` | `OPENAI_API_KEY` | `ANTHROPIC_API_KEY` |

### Missing API key

```
Error: API key not found for agent "claude"
```

Set the environment variable:

```bash
export ANTHROPIC_API_KEY=sk-ant-...
```

For persistent configuration, add it to your shell profile (`~/.bashrc`, `~/.zshrc`, etc.).

### Wrong key for agent

If using `aider`, note that it checks `OPENAI_API_KEY` first and falls back to `ANTHROPIC_API_KEY`. Make sure at least one is set.

---

## Ralph Loop Issues

### PRD file not found

```
Error: open prd.json: no such file or directory
```

Initialize the project first or specify the correct path:

```bash
# Create default PRD
agentbox init

# Or point to your PRD file
agentbox ralph --prd path/to/my-prd.json
```

### Stuck tasks / no progress

If the Ralph loop keeps running the same task without completing it:

1. **Check task descriptions** — The agent may not understand what "done" means. Add clearer acceptance criteria to the task description in `prd.json`.

2. **Check the stop signal** — The agent must output the configured stop signal (default: `<promise>COMPLETE</promise>`) for the loop to mark a task as complete. Verify `ralph.stop_signal` in `agentbox.yaml` hasn't been changed to something the agent won't produce.

3. **Check `progress.txt`** — Review learnings from previous iterations. The agent may be hitting a recurring error.

4. **Manually advance** — Edit `prd.json` directly to set a task's `status` to `"completed"` and move on.

### Max iterations reached

```
Reached maximum iterations (10)
```

The loop stops after `max_iterations` to prevent runaway execution. Options:

- Increase the limit: `agentbox ralph --max-iterations 20`
- Check why tasks aren't completing (see "Stuck tasks" above)
- Run `agentbox status` to see progress and decide whether to continue

### Quality check failures

Quality checks run after each task completion. If they fail, the loop continues but logs the failure. Common causes:

- **Test failures** — The agent's changes broke existing tests. Review the output and fix manually, or let the next iteration address it.
- **Type errors** — The agent introduced type errors. Check the quality check output in the terminal.
- **Command not allowed** — See "Quality Check Allowlist" below.

---

## Quality Check Allowlist

Quality check commands in `agentbox.yaml` are validated against an allowlist. Only the following command prefixes are accepted:

### Allowed Commands

| Category | Commands |
|----------|----------|
| **JavaScript/TypeScript** | `npm`, `npx`, `pnpm`, `yarn`, `bun` |
| **Go** | `go` |
| **Rust** | `cargo`, `rustc` |
| **Python** | `python`, `python3`, `pytest`, `pip` |
| **Build tools** | `make`, `gradle`, `mvn` |
| **Linters/Formatters** | `eslint`, `prettier`, `tsc`, `jest`, `vitest`, `mocha` |

### How Validation Works

The first word of each command is extracted and checked against the allowlist. Only the base command name matters — arguments and flags are not restricted.

### Valid Commands

```yaml
quality_checks:
  - name: test
    command: npm test
  - name: typecheck
    command: tsc --noEmit
  - name: lint
    command: eslint src/
  - name: build
    command: go build ./...
  - name: pytest
    command: pytest tests/ -v
  - name: cargo-test
    command: cargo test
```

### Rejected Commands

```yaml
quality_checks:
  # Rejected — "bash" is not in the allowlist
  - name: custom
    command: bash scripts/check.sh

  # Rejected — "curl" is not in the allowlist
  - name: api-check
    command: curl http://localhost:3000/health

  # Rejected — "sh" is not in the allowlist
  - name: shell
    command: sh -c "echo test"
```

### Workarounds

If you need to run a command not on the allowlist:

1. **Wrap it in a Makefile target** — `make` is allowed, so `make my-check` works:
   ```makefile
   # Makefile
   my-check:
       bash scripts/check.sh
   ```
   ```yaml
   quality_checks:
     - name: custom-check
       command: make my-check
   ```

2. **Use an npm/pnpm script** — `npm` and `pnpm` are allowed:
   ```json
   // package.json
   { "scripts": { "check": "bash scripts/check.sh" } }
   ```
   ```yaml
   quality_checks:
     - name: custom-check
       command: npm run check
   ```
