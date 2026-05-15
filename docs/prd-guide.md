# PRD Guide

The PRD (Product Requirements Document) is the core input for the Ralph loop. It defines tasks for the AI agent to complete, tracks progress, and persists state across iterations.

## PRD Schema

A PRD file is a JSON document with the following structure:

### Top-Level Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | Yes | Project or feature name |
| `description` | `string` | Yes | High-level description of what to build |
| `tasks` | `Task[]` | Yes | Ordered list of tasks |
| `metadata` | `PRDMeta` | No | Auto-populated progress metadata |

### Task Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | `string` | Yes | Unique task identifier |
| `title` | `string` | Yes | Short task title |
| `description` | `string` | Yes | Detailed description of what to do |
| `status` | `string` | Yes | Task status (see below) |
| `priority` | `int` | No | Priority level (lower = higher priority) |
| `depends_on` | `string[]` | No | IDs of tasks that must complete first |
| `subtasks` | `Task[]` | No | Nested subtasks (same structure) |
| `learnings` | `string` | No | Notes captured during execution |
| `completed_at` | `string` (ISO 8601) | No | Timestamp when completed (e.g., `"2025-01-15T10:30:00Z"`) |

### Status Values

| Status | Meaning |
|--------|---------|
| `pending` | Not started, waiting to be picked up |
| `in_progress` | Currently being worked on by an agent |
| `completed` | Successfully finished |
| `blocked` | Cannot proceed (dependencies not met) |

### Metadata Fields

Metadata is automatically updated by the Ralph loop:

| Field | Type | Description |
|-------|------|-------------|
| `created_at` | `string` | When the PRD was created |
| `updated_at` | `string` | Last modification time |
| `total_tasks` | `int` | Total task count |
| `completed` | `int` | Number of completed tasks |
| `in_progress` | `int` | Number of in-progress tasks |
| `pending` | `int` | Number of pending tasks |

## Task Dependencies

The `depends_on` field lists task IDs that must be completed before a task can start.

### How `NextTask()` Resolves Tasks

1. Collects all completed task IDs (including completed subtasks)
2. Iterates tasks in order, skipping any that are not `pending` or `in_progress`
3. For each candidate task, checks if **all** `depends_on` IDs are in the completed set — if not, the task is blocked and skipped
4. If a task has subtasks, recurses into them and returns the first available subtask
5. Returns `nil` when no unblocked, incomplete tasks remain (PRD is complete)

This means task ordering in the `tasks` array matters — earlier tasks are picked up first when dependencies are equal.

## Writing Good Tasks

Tasks are used to generate prompts for AI agents. Well-written tasks lead to better agent output.

### Guidelines

- **Atomic scope** — Each task should do one thing. "Add login endpoint" is better than "Build the authentication system."
- **Clear acceptance criteria** — Describe what "done" looks like. "Create a `/login` POST endpoint that accepts `{email, password}` and returns a JWT" gives the agent a concrete target.
- **Include context** — Mention relevant files, libraries, or patterns. "Use the existing `UserService` in `src/services/user.ts`" helps the agent find its way.
- **Order by dependency** — Place foundation tasks first. Database schema before API endpoints, API endpoints before UI.
- **Keep descriptions agent-friendly** — Write as if instructing a developer. The description is inserted directly into the agent's prompt.

### What Gets Sent to the Agent

The Ralph loop builds a prompt from the current task:

```
You are working on: <PRD name>

Current task:
ID: <task.id>
Title: <task.title>
Description: <task.description>

Instructions:
1. Complete the task described above
2. Make small, focused changes
3. Ensure your changes are complete and tested
4. When the task is FULLY complete, output: <stop_signal>

Important: Only output the completion signal when the task is truly done.
```

The default stop signal is `<promise>COMPLETE</promise>` (configurable via `ralph.stop_signal` in `agentbox.yaml`).

### Learnings

After each iteration, the Ralph loop scans agent output for lines starting with `learning:`, `note:`, or `important:` (case-insensitive). These are extracted and saved to the task's `learnings` field and appended to `progress.txt`, making them available to future iterations.

## Example PRD

```json
{
  "name": "User Authentication",
  "description": "Add email/password authentication with JWT tokens",
  "tasks": [
    {
      "id": "task-1",
      "title": "Create user database schema",
      "description": "Create a users table with columns: id (UUID primary key), email (unique, not null), password_hash (not null), created_at, updated_at. Add a migration file in src/db/migrations/.",
      "status": "pending",
      "priority": 1
    },
    {
      "id": "task-2",
      "title": "Implement user registration endpoint",
      "description": "Create POST /api/register that accepts {email, password}, validates input with Zod, hashes password with bcrypt, inserts into users table, and returns {id, email}. Return 400 for invalid input, 409 for duplicate email.",
      "status": "pending",
      "priority": 2,
      "depends_on": ["task-1"]
    },
    {
      "id": "task-3",
      "title": "Implement login endpoint",
      "description": "Create POST /api/login that accepts {email, password}, verifies credentials, and returns a JWT token with 24h expiry. Return 401 for invalid credentials. Use the jsonwebtoken package.",
      "status": "pending",
      "priority": 2,
      "depends_on": ["task-1"]
    },
    {
      "id": "task-4",
      "title": "Add auth middleware",
      "description": "Create an Express middleware that extracts the JWT from the Authorization header (Bearer scheme), verifies it, and attaches the user to req.user. Return 401 for missing/invalid tokens. Apply to all /api/* routes except /api/register and /api/login.",
      "status": "pending",
      "priority": 3,
      "depends_on": ["task-3"]
    },
    {
      "id": "task-5",
      "title": "Add authentication tests",
      "description": "Write integration tests covering: successful registration, duplicate email rejection, successful login, invalid credentials, protected route access with valid token, protected route rejection without token. Use supertest.",
      "status": "pending",
      "priority": 4,
      "depends_on": ["task-2", "task-3", "task-4"]
    }
  ]
}
```

**Notes on this example:**
- Tasks are ordered by dependency — schema first, then endpoints, then middleware, then tests
- `task-2` and `task-3` both depend on `task-1` but not each other, so the Ralph loop picks `task-2` first (array order)
- `task-5` depends on multiple tasks and runs last
- Descriptions include specific file paths, packages, HTTP methods, and expected behavior
