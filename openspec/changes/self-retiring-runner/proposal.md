## Why

`line run` currently starts a polling daemon that ticks on a timer forever until killed. Users must explicitly start it (`line run` or via the `line-start` skill) and it idles between commits, wasting a background process. The ideal user experience is invisible: commits trigger processing automatically, and the runner exits when there's nothing left to do.

## What Changes

- A post-commit hook automatically spawns a background runner when a commit lands
- The runner processes the station line, then waits one grace period for more work before exiting
- No explicit `line start` / `line stop` lifecycle — the runner is ephemeral and self-managing
- A hidden `line trigger` subcommand handles the hook-to-runner handoff (write trigger file, check for existing runner, spawn if needed)
- IPC via trigger file (`.line/trigger`) — hook writes commit hash, runner checks mod time after each grace period
- Single-instance guard via PID file (`.line/runner.pid`) — reuses the existing `IsProcessAlive` pattern
- `line run` (without `--once`) now enters the self-retiring runner loop instead of polling forever
- `line init` installs a post-commit hook alongside the existing pre-commit hook

## Capabilities

### New Capabilities
- `runner-lifecycle`: Specifies how the runner is triggered, how it guards against duplicate instances, how it decides to continue or exit, and how the post-commit hook integrates with `line init`

### Modified Capabilities
- `assembly-line`: The "Branch Watching" requirement changes — the runner no longer polls indefinitely but processes on trigger and exits after a grace period with no new work

## Impact

- `internal/engine/runner.go`: New file — runner loop, PID file management, trigger file management
- `internal/cli/run.go`: Replace `runDaemon()` with call to `engine.RunnerLoop`; remove `reloadConfig`
- `internal/cli/trigger.go`: New file — hidden `line trigger` subcommand
- `internal/cli/init.go`: Add post-commit hook installation using same sentinel pattern as pre-commit
- `internal/assets/skills/line-start/SKILL.md`: Simplify to reflect auto-triggered model
- `.line/trigger` and `.line/runner.pid`: New runtime files (already gitignored via `.line/`)
- Users no longer need to manually start or stop the daemon
