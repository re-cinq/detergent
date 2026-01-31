
# Agent Instructions

## Tool Responsibilities

| Tool | Purpose | Scope |
|------|---------|-------|
| **OpenSpec** | Design before coding | Single proposal lifecycle |
| **Beads (bd)** | Track work state | Project-wide, persistent |

### Decision Tree

```
New work request?
├─ New capability / breaking change / architecture?
│   └─ Create OpenSpec proposal → implement via tasks.md
├─ Bug fix restoring intended behavior?
│   └─ Just fix it (no proposal needed)
├─ Work discovered during implementation?
│   └─ Create Beads issue (bd create)
├─ Can't finish this session?
│   └─ Create Beads issue for handoff
└─ Quick fix / typo / config?
    └─ Just do it
```

### Key Principle

**OpenSpec `tasks.md` = what to build. Beads = state across sessions.**

- Don't duplicate tasks.md into Beads—that's redundant
- Use Beads for emergent work (bugs, blockers, follow-ups)
- Use Beads when you need dependencies between items

## Beads Quick Reference

Run `bd prime` for full workflow context.

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --status in_progress  # Claim work
bd close <id>         # Complete work
bd sync               # Sync with git
bd create --title="..." --type=bug|task|feature  # New issue
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
