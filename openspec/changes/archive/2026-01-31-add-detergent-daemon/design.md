# Design: Assembly Line Daemon

## Context

We need to orchestrate multiple coding agents, each focused on a single quality station, in a way that:
- Preserves the intent of upstream changes
- Provides a clear audit trail
- Allows parallel execution where possible
- Recovers gracefully from failures

## Goals / Non-Goals

**Goals:**
- Simple configuration via YAML
- Git-native operation (no special server)
- Clear audit trail via commits and notes
- Idempotent operation (safe to restart)

**Non-Goals:**
- Real-time streaming (polling is sufficient)
- Multi-repo orchestration (single repo focus)
- Agent implementation (delegates to external CLI)

## Decisions

### Decision: Polling vs Webhooks
**Choice:** Polling with configurable interval

**Rationale:**
- Works with any Git remote (no webhook setup)
- Simpler to implement and debug
- Suitable for code review cadence (not real-time)

**Alternatives considered:**
- Webhooks: More complex setup, requires server exposure
- File watching: Only works locally, misses remote pushes

---

### Decision: Worktrees for Isolation
**Choice:** One git worktree per station

**Rationale:**
- Git-native isolation (no container overhead)
- Allows parallel execution without locks
- Clean separation of station state

**Alternatives considered:**
- Stash/restore: Race conditions, complex state management
- Separate clones: Disk space overhead, sync complexity

---

### Decision: Git Notes for Audit
**Choice:** Git notes for no-change reviews

**Rationale:**
- Preserves original commit hashes (clean history)
- Queryable via `git log --show-notes`
- No branch pollution

**Alternatives considered:**
- Empty commits: Pollutes history
- External database: Loses Git integration

---

### Decision: Fast-Forward for No-Change
**Choice:** Fast-forward output branch when no changes needed

**Rationale:**
- Downstream stations see commits arrive (triggers their processing)
- Original commits pass through with hashes intact
- Combined with git notes, provides complete audit trail

---

### Decision: Commit Message Format
**Choice:** `[{STATION_NAME}] {summary}` with `Triggered-By:` trailer

**Rationale:**
- Tag at start is easy to grep/filter
- Trailer is Git convention (like `Signed-off-by`)
- Enables downstream agents to see station line

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| Agent makes breaking changes | Implicit priority (upstream wins), human review at line end |
| Polling misses rapid commits | Batch processing handles multiple commits per poll |
| Worktree disk usage | Cleanup command, shallow worktrees option |
| Agent timeout/hang | Configurable timeout, kill after threshold |

## Migration Plan

N/A - new capability, no migration needed.

## Open Questions

1. Should there be a "human checkpoint" station type that pauses for review?
2. Should the final output branch be auto-merged to a target branch?
3. How to handle merge conflicts if someone pushes to an output branch manually?
