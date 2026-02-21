## 1. Runner Infrastructure

- [ ] 1.1 Create `internal/engine/runner.go` with trigger file functions: `TriggerPath(repoDir) string`, `WriteTrigger(repoDir, commitHash) error`, `ReadTrigger(repoDir) (string, time.Time, error)`
- [ ] 1.2 Add PID file functions: `PIDPath(repoDir) string`, `WritePID(repoDir) error`, `ReadPID(repoDir) int`, `RemovePID(repoDir)`, `IsRunnerAlive(repoDir) bool`
- [ ] 1.3 Implement `RunnerLoop(ctx, configPath, cfg, repoDir) error` — write PID (defer remove), guard against duplicate runner, loop: record trigger mod time → `RunOnceWithLogs` → sleep grace period → check trigger mod time → loop or exit
- [ ] 1.4 Config hot-reload each cycle (move `reloadConfig` logic into `RunnerLoop`, remove from `run.go`)
- [ ] 1.5 Unit tests in `internal/engine/runner_test.go`: WriteTrigger/ReadTrigger round-trip, ReadTrigger on missing file, WritePID/ReadPID round-trip, IsRunnerAlive for missing PID, RemovePID cleanup, trigger mod time advances on re-write

## 2. CLI Integration

- [ ] 2.1 Modify `internal/cli/run.go`: replace `runDaemon()` with call to `engine.RunnerLoop`, remove `reloadConfig` function, remove unused imports, update command description
- [ ] 2.2 Create `internal/cli/trigger.go`: hidden `line trigger` subcommand — resolve repo dir, get HEAD via `git rev-parse HEAD`, call `engine.WriteTrigger`, check `engine.IsRunnerAlive`, if not alive spawn `line run --path <configPath>` detached (stdin/stdout/stderr → /dev/null, `cmd.Process.Release()`)
- [ ] 2.3 Acceptance test for `line trigger`: writes trigger file, spawns runner when none alive

## 3. Post-Commit Hook

- [ ] 3.1 Add post-commit hook constants and `initPostCommitHook(repoDir)` to `internal/cli/init.go` using same sentinel pattern as pre-commit: `# BEGIN line runner` / `# END line runner` markers, calls `line trigger >/dev/null 2>&1`
- [ ] 3.2 Wire into `initCmd.RunE`: install post-commit hook when stations are configured (`len(cfg.Stations) > 0`)
- [ ] 3.3 Acceptance tests in `test/acceptance/init_hook_test.go`: post-commit hook installed when stations configured, idempotent re-installation, not installed when no stations, injects into existing post-commit hook

## 4. Self-Retiring Behavior

- [ ] 4.1 Rewrite `test/acceptance/daemon_test.go`: runner processes commits then exits after grace period with no new work
- [ ] 4.2 Acceptance test: runner stays alive and processes new work when trigger arrives during grace period
- [ ] 4.3 Acceptance test: PID file cleaned up after runner exits
- [ ] 4.4 Acceptance test for trigger command: `line trigger` writes trigger file and spawns runner, post-commit hook integration via `line init`

## 5. Skill Update

- [ ] 5.1 Update `internal/assets/skills/line-start/SKILL.md`: explain auto-triggered model, simplify to note that commits trigger processing automatically, retain `line run --once` for debugging
