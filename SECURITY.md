# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in agentbox, please report it responsibly:

1. **Do not** open a public GitHub issue for security vulnerabilities
2. Email details to the maintainers (see repository for contact info)
3. Include steps to reproduce the vulnerability
4. Allow reasonable time for a fix before public disclosure

## Security Model

Agentbox provides defense-in-depth isolation for running AI coding agents:

### Container Isolation

| Resource | Protection |
|----------|------------|
| Filesystem | Only `/workspace` (your project) accessible |
| Network | Disabled by default (`--allow-network` to enable) |
| Processes | Separate PID namespace |
| User | Runs as non-root `agent` user (UID 1000) |
| Docker | No access to host docker.sock |

### Host Exposure

The following are exposed to containers (read-only):

- **SSH keys** (`~/.ssh`): For git operations requiring authentication
- **Git config** (`~/.gitconfig`): For commit author information
- **API keys**: Passed via environment variables

### What Agents Can Do

Within the mounted project directory, agents can:

- Read, create, modify, and delete any file
- Execute commands (tests, builds, etc.)
- Make commits to git (if auto-commit enabled)

### Quality Check Commands

Quality check commands in `agentbox.yaml` are validated against a whitelist of known-safe build tools (npm, go, cargo, python, etc.). Arbitrary shell commands are rejected.

## Known Limitations

### Not Protected Against

1. **Malicious project code**: If your project contains malicious code that runs during tests/builds, it will execute
2. **Resource exhaustion**: Set memory/CPU limits in config to prevent runaway processes
3. **Expensive API calls**: Agents may make many API calls; monitor your usage
4. **Data in project directory**: Agents can read/modify all project files

### Trust Requirements

- **Config files**: Only use `agentbox.yaml` from trusted sources
- **PRD files**: Task definitions are used to prompt agents
- **Docker images**: Use official agentbox images or build your own from provided Dockerfiles

## Best Practices

1. **Review changes**: Always review agent-generated code before committing to production
2. **Use git branches**: Run agents on feature branches, not main
3. **Limit network access**: Keep default `network: none` unless API access required
4. **Set resource limits**: Configure memory and CPU limits
5. **Monitor API usage**: Watch for unexpected API consumption
6. **Audit quality checks**: Only add commands you trust to quality_checks

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.x.x   | ✓ (development) |

## Security Updates

Security updates will be released as patch versions. Subscribe to repository releases for notifications.
