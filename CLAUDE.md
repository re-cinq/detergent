<!-- OPENSPEC:START -->

# OpenSpec Instructions

These instructions are for AI assistants working in this project.

Always open `@/openspec/AGENTS.md` when the request:

- Mentions planning or proposals (words like proposal, spec, change, plan)
- Introduces new capabilities, breaking changes, architecture shifts, or big performance/security work
- Sounds ambiguous and you need the authoritative spec before coding

Use `@/openspec/AGENTS.md` to learn:

- How to create and apply change proposals
- Spec format and conventions
- Project structure and guidelines

Keep this managed block so 'openspec update' can refresh the instructions.

<!-- OPENSPEC:END -->

# Agent Instructions

## Tool Responsibilities

| Tool | Use For | Examples |
|------|---------|----------|
| **OpenSpec** | High-level specs & proposals | New capabilities, breaking changes, architecture decisions |
| **Beads (bd)** | Issue tracking & work items | Bugs, tasks, features, discovered work |

**Key distinction:**
- OpenSpec `tasks.md` = implementation checklist scoped to one proposal
- Beads issues = project-wide work tracking with dependencies

**When proposing changes:** Use OpenSpec for the spec/design, then optionally create Beads issues for implementation tracking if the work spans multiple sessions or has dependencies.

## Issue Tracking (Beads)

This project uses **bd** (beads) for issue tracking. Run `bd prime` for full workflow context.

## Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --status in_progress  # Claim work
bd close <id>         # Complete work
bd sync               # Sync with git
```

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Use `bd create` for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work with `bd close <id>`
4. **Commit & sync**:
   ```bash
   git add <files>
   bd sync
   git commit -m "..."
   ```
5. **Push or merge** (check `bd prime` for branch-specific guidance):
   - **Tracked branches**: `git push` (verify with `git status`)
   - **Ephemeral branches**: Merge to main locally
6. **Hand off** - Provide context for next session

**CRITICAL RULES:**

- Work is NOT complete until changes are committed AND synced
- NEVER stop mid-session without committing - that leaves work stranded
- NEVER say "ready to commit when you are" - YOU must commit
- Check `bd prime` for the current branch's workflow
