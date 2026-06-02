# Ticket workflow

The canonical process for any change to this repo. **Source of truth.**
`CLAUDE.md` and the global rules under `~/.dotfiles/.claude/rules/` reference
this doc. If you change the workflow, change it here first.

---

## The seven steps

| # | Step | Automation |
|---|---|---|
| 1 | Worktree + branch off latest `main`; read the ticket fully | `/start-ticket <N>` |
| 2 | Research + clarify: read relevant code, break into testable units | manual |
| 3 | Implement with TDD (Red → Green → Refactor) | manual |
| 4 | Code review via the `code-reviewer` subagent | `/finish-ticket` |
| 5 | Fix critical and significant findings; re-review if fixes were substantial | `/finish-ticket` re-fires reviewer |
| 6 | Open the PR — Conventional-Commit title, `Closes #NN` in body | `/finish-ticket` |
| 7 | When hosted CI is green, merge and clean up | `/ship-ticket <P>` |

Steps are sequential. No skipping. Step 7 is **gated** — see "Self-merge
authority" below.

---

## Per-PR checklist (mandatory)

Every PR clears this five-step checklist before it ships. This is the
mandatory checklist from `~/.dotfiles/.claude/rules/local-ci-and-admin-merge.md`,
restated here with project-specific commands.

1. **Code review** — invoke the `code-reviewer` subagent on the diff.
2. **Address findings** — fix every critical and significant item; defer
   minor items to the PR body.
3. **Run all CI jobs locally** — mirrors the two hosted-CI jobs. All must pass:
   ```bash
   make fmt    # format check (goimports + go fmt) — part of ci job
   make ci     # lint + go test -race + coverage ≥ 65% — mirrors ci job
   make smoke  # build + binary help/version — mirrors smoke-test job
   ```
4. **Apply fixes** for anything that fails. If a fix touches behaviour
   outside the original review's scope, restart the checklist from step 1.
5. **Re-review on low confidence** — if step 4 was non-trivial, run
   `code-reviewer` again before declaring the PR ready.

These five steps run **every time**, regardless of hosted-CI status.
`/finish-ticket` automates checklist steps 1–2 and 5; the make commands above are checklist step 3.

### "Substantial fix" heuristic for step 5

Re-fire the reviewer if **any** of these hold for the diff between first
review and current state:

- New files added that weren't in the original review.
- More than 50 net lines added since first review.
- Any new exported symbol introduced.
- The fix landed in an area flagged critical or significant by the first pass.

If none of these hold, treat the fix as minor and skip re-review.

---

## Self-merge authority

By default in `~/.dotfiles/.claude/rules/collaboration.md`, agents do not
`gh pr merge` — the user merges. **This project uses the global default.**
There is no project-specific relaxation.

Standard flow after `/finish-ticket`:
1. Share the PR URL and CI status.
2. Stop — the user merges on their own schedule.

---

## Cleanup

After merge, `/ship-ticket` does:

1. `gh issue comment <N>` on every ticket the PR closes — what shipped,
   anything to watch for, link to PR.
2. `git worktree remove ../ab-<N>-<slug>` from the main repo. Never
   `--force` — if uncommitted changes exist, stop and report.
3. `git branch -d <type>/<N>-<slug>` to drop the local branch.

Cleanup runs in the same session as the merge.

---

## Branches, commits, naming

- **Worktree:** `../ab-<N>-<slug>` (the `ab-` prefix avoids collisions with
  sibling repos in the workspace — new convention as of this doc). Without an
  issue number: `../ab-<slug>`.
- **Branch:** `<type>/<N>-<slug>` where `<type>` ∈ `feat`, `fix`, `chore`,
  `refactor`, `test`, `docs`. `<N>` is the GitHub issue number. Without an
  issue: `<type>/<slug>`.
- **Commits:** Conventional-Commit format, `<type>(<scope>): <short summary>`.
  Scope is the package name (`ralph`, `supervisor`, `agent`, `store`, `cli`,
  `container`, …). For TDD work, separate the failing-test commit from the
  implementation commit:
  ```
  test(ralph): add failing test for empty PRD handling
  feat(ralph): handle empty PRD gracefully
  ```
- **PR title:** Conventional-Commit format, under 70 characters.
- **PR body:** must contain `Closes #NN` on its own line.

---

## Where to find what

| Question | File |
|---|---|
| Per-PR checklist (canonical) | `~/.dotfiles/.claude/rules/local-ci-and-admin-merge.md` |
| Default merge authority across projects | `~/.dotfiles/.claude/rules/collaboration.md` |
| Generic 9-step end-to-end process | `~/.dotfiles/.claude/rules/end-to-end-process.md` |
| TDD patterns, bug-fix process | `~/.dotfiles/.claude/rules/development-workflow.md` |
| This project's stack, naming, test commands | [`../CLAUDE.md`](../CLAUDE.md) |
| CI jobs and coverage threshold | [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) |
| Contributing guide (Make targets, agent interface) | [`../CONTRIBUTING.md`](../CONTRIBUTING.md) |

---

## One last thing

If you're an agent and the workflow seems to want you to do something
risky — sweep multiple PRs, force-push, bypass the checklist — **stop
and ask the user**. The conditions in this doc are deliberately tight;
when in doubt, default to the more conservative read.
