## Context

Assembly Line currently runs as a long-lived polling daemon (`line run`). Users must explicitly start it, either by running `line run` in a terminal or using the `line-start` skill to launch it as a Claude Code background task. Between commits, the daemon idles — consuming a process slot and terminal/background task for no reason. If the user forgets to start it, commits go unprocessed. If the daemon crashes, nothing restarts it.

The ideal model: commits automatically trigger processing. The runner does its job and gets out of the way.

## Goals / Non-Goals

**Goals:**
- Automatically process the station line when commits land, with no user intervention
- Runner exits after a grace period with no new work (no zombie processes)
- Single-instance guard prevents duplicate runners from stomping each other
- Post-commit hook installed via `line init`, same patterns as existing pre-commit hook
- Config hot-reload preserved (reloaded each cycle, same as current behavior)
- `line run --once` unchanged for manual/debugging use

**Non-Goals:**
- Watching for non-commit events (branch creation, push, etc.)
- Cross-repo triggering or remote webhook integration
- Systemd/launchd service management
- Log rotation for runner output (existing log management is sufficient)
- Real-time notification when runner starts/stops

## Decisions

### 1. IPC mechanism: Trigger file with mod-time checking

**Decision:** The post-commit hook writes the HEAD commit hash to `.line/trigger`. The runner checks the file's modification time after each grace period to decide whether new work arrived.

**Rationale:**
- Atomic at the filesystem level — a write either completes or doesn't
- No dependencies on OS-specific IPC (pipes, sockets, signals)
- Portable across macOS and Linux
- Simple to debug (`cat .line/trigger`, `stat .line/trigger`)
- Worst-case race (hook writes between runner's check and exit) results in a missed trigger, but the next commit spawns a fresh runner

**Alternatives considered:**
- Unix signals (SIGUSR1): Requires knowing the runner PID from the hook, adds signal handling complexity, not portable to Windows
- Named pipe / Unix socket: More complex setup and teardown, overkill for "is there new work?"
- File lock with advisory locking: Solves a problem we don't have (the trigger file doesn't need mutual exclusion)

### 2. Single-instance guard: PID file with `IsProcessAlive`

**Decision:** Runner writes its PID to `.line/runner.pid` on startup. Before writing, it checks if an existing PID file points to a living process (using the existing `IsProcessAlive` function from `state.go`). If alive, the new runner exits immediately.

**Rationale:**
- Reuses proven pattern already in the codebase (`IsProcessAlive` uses `Signal(0)`)
- Handles stale PID files automatically (process died, PID recycled → new process won't match)
- Simple cleanup: delete PID file on exit (deferred)
- No external dependencies

**Alternatives considered:**
- File locks (flock/fcntl): More robust against PID recycling but adds OS-specific code and complexity
- Socket binding: Would prevent any duplicate, but adds networking dependency
- No guard: Risk of two runners processing simultaneously (mostly harmless due to last-seen tracking, but wasteful)

### 3. Grace period: Reuse `poll_interval` config

**Decision:** After processing the station line, the runner sleeps for `poll_interval` (from config), then checks the trigger file's mod time. If unchanged, it exits. If advanced, it loops.

**Rationale:**
- No new config field needed — `poll_interval` already represents "how long to wait between checks"
- Semantically correct: the grace period is the same duration a user would expect between processing cycles
- Config hot-reload happens at the top of each cycle, so a changed `poll_interval` takes effect naturally

**Alternatives considered:**
- Dedicated `grace_period` config: Extra complexity for marginal value; poll_interval is the right semantic
- Fixed grace period (e.g., 30s): Less flexible, ignores user's configured timing preference
- No grace period (exit immediately after processing): Would miss rapid consecutive commits, spawning many short-lived runners

### 4. Runner loop placement: `engine.RunnerLoop`

**Decision:** The core runner loop lives in `internal/engine/runner.go`, called by `internal/cli/run.go`. The CLI layer sets up the command and passes config; the engine layer owns the loop logic.

**Rationale:**
- Consistent with existing architecture: `engine.RunOnce`, `engine.RunOnceWithLogs` are engine-layer functions called by CLI
- Keeps CLI thin (parse args, load config, delegate)
- Runner infrastructure (PID files, trigger files) lives alongside state management in the engine package
- Testable without CLI overhead

**Alternatives considered:**
- Loop in CLI layer: Would make `run.go` fat and harder to test
- Separate `runner` package: Over-engineering; the engine package is the right home

### 5. Hook spawns `line run` (not a custom binary/script)

**Decision:** The `line trigger` command spawns `line run --path <configPath>` as a detached background process when no runner is alive. The spawned `line run` enters `RunnerLoop`.

**Rationale:**
- Reuses existing `line run` binary — no separate daemon binary to build/distribute
- `line run` already handles config loading, repo resolution, signal handling
- Detached process (stdin/stdout/stderr → `/dev/null`, `cmd.Process.Release()`) survives hook exit
- The hook is fast: write file, check PID, maybe spawn — all sub-millisecond

**Alternatives considered:**
- Hook runs processing inline: Would block the commit, terrible UX
- Hook spawns a shell script: Extra file to manage, no benefit over spawning the Go binary directly
- Separate `line daemon` command: Needless command proliferation

### 6. Post-commit hook installation: Same sentinel pattern as pre-commit

**Decision:** `line init` installs a post-commit hook using the same `BEGIN/END` sentinel marker pattern as the existing pre-commit gate hook. The hook calls `line trigger` (with output suppressed).

**Rationale:**
- Proven pattern — already battle-tested with pre-commit hooks
- Idempotent re-installation
- Plays nice with existing hooks (injects rather than replaces)
- Users can inspect and understand the hook easily

**Alternatives considered:**
- Separate hook manager: Over-engineering for two hooks
- Core.hooksPath config: Git supports this but it's rarely used and would break existing hook setups

## Risks / Trade-offs

**[Risk] Race between hook and runner exit**
→ Mitigation: Hook writes trigger file *first*, then checks PID. If runner exits between those two operations, the hook spawns a new runner that immediately sees the fresh trigger. Worst case: one extra runner spawns and exits after finding no new work (harmless).

**[Risk] Multiple rapid commits spawn multiple runners**
→ Mitigation: `RunnerLoop` checks `IsRunnerAlive` before writing its PID. Second runner sees the first and exits immediately. Even if both pass the check simultaneously (tiny window), last-seen tracking makes double-processing a no-op.

**[Risk] Runner crashes, PID file left behind**
→ Mitigation: `IsProcessAlive` detects stale PID files (process no longer running). Next hook invocation spawns a fresh runner. PID file is always cleaned up on normal exit via `defer RemovePID`.

**[Trade-off] No explicit start/stop commands**
→ Mitigation: `line run --once` still works for debugging. `line run` (without --once) enters the self-retiring loop, so users can still start it manually if needed. The mental model is simpler: commits trigger processing, period.

**[Trade-off] Runner may exit just before a commit, causing brief spawn latency**
→ Mitigation: Spawn latency is the time to execute `line run` (Go binary startup, ~50ms). This is imperceptible to users. The alternative (keeping the runner alive longer) wastes more resources than it saves.

## Migration Plan

No breaking changes to user-facing config. The `poll_interval` field gains a secondary meaning (grace period) that is backwards-compatible. Users who previously ran `line run` manually will find it now auto-exits when idle — they should rely on the post-commit hook instead (installed via `line init`).

Existing repos need to re-run `line init` to install the post-commit hook. The `line-start` skill will be updated to explain the new model.

## Open Questions

None — all design decisions are resolved.
