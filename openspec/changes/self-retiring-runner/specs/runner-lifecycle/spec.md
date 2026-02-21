## ADDED Requirements

### Requirement: Post-commit hook triggers runner
The system SHALL provide a post-commit hook that triggers station-line processing after each commit. The hook MUST be installed by `line init` when stations are configured.

#### Scenario: Hook installed by line init
- **WHEN** `line init` runs and the config has stations defined
- **THEN** a post-commit hook is installed in `.git/hooks/post-commit`
- **AND** the hook contains sentinel markers (`# BEGIN line runner` / `# END line runner`)
- **AND** the hook calls `line trigger` with output suppressed

#### Scenario: Hook installation is idempotent
- **WHEN** `line init` runs and the post-commit hook already contains the sentinel markers
- **THEN** the hook is not modified
- **AND** the sentinel block appears exactly once

#### Scenario: Hook injected into existing post-commit hook
- **WHEN** `line init` runs and a post-commit hook already exists without the sentinel
- **THEN** the runner block is injected into the existing hook
- **AND** the original hook content is preserved

#### Scenario: No hook when no stations configured
- **WHEN** `line init` runs and no stations are defined in config
- **THEN** no post-commit hook is installed for the runner

---

### Requirement: Trigger command manages runner lifecycle
The system SHALL provide a hidden `line trigger` subcommand that writes a trigger file and ensures a runner is alive. This command is called by the post-commit hook.

#### Scenario: Trigger writes commit hash
- **WHEN** `line trigger` runs
- **THEN** the HEAD commit hash is written to `.line/trigger`
- **AND** the file's modification time is updated

#### Scenario: Trigger spawns runner when none alive
- **WHEN** `line trigger` runs and no runner process is alive
- **THEN** a `line run` process is spawned as a detached background process
- **AND** the spawned process's stdin, stdout, and stderr are redirected to /dev/null

#### Scenario: Trigger skips spawn when runner alive
- **WHEN** `line trigger` runs and a runner process is already alive (per PID file)
- **THEN** no new runner is spawned
- **AND** the trigger file is still written (runner will pick it up on next cycle)

---

### Requirement: Runner self-retires after idle grace period
The system SHALL run the station line and then wait one grace period (equal to `poll_interval`) for new work. If no new trigger arrives during the grace period, the runner exits.

#### Scenario: Runner exits when no new work
- **WHEN** the runner finishes processing and sleeps for the grace period
- **AND** no new trigger file write occurs during that period
- **THEN** the runner exits cleanly

#### Scenario: Runner continues when new work arrives
- **WHEN** the runner finishes processing and sleeps for the grace period
- **AND** a new commit triggers a trigger file write during that period
- **THEN** the runner processes the new work
- **AND** starts a new grace period after processing

#### Scenario: Multiple rapid commits
- **WHEN** several commits land in quick succession
- **THEN** each commit overwrites the trigger file
- **AND** the runner processes the station line (which handles all new commits via last-seen tracking)
- **AND** exits after a grace period with no further commits

---

### Requirement: Single-instance runner guard
The system SHALL prevent multiple runners from processing simultaneously using a PID file (`.line/runner.pid`).

#### Scenario: PID file written on startup
- **WHEN** the runner starts
- **THEN** it writes its PID to `.line/runner.pid`

#### Scenario: PID file cleaned up on exit
- **WHEN** the runner exits (normally)
- **THEN** the PID file is removed

#### Scenario: Duplicate runner exits immediately
- **WHEN** a second runner starts and detects a living process at the PID in the PID file
- **THEN** the second runner exits immediately without processing

#### Scenario: Stale PID file detected
- **WHEN** a runner starts and the PID file exists but the process is not alive
- **THEN** the runner treats this as no existing runner
- **AND** writes its own PID and proceeds with processing

---

### Requirement: Config hot-reload in runner loop
The system SHALL reload configuration from disk at the start of each processing cycle, preserving the existing hot-reload behavior.

#### Scenario: Config reload on each cycle
- **WHEN** the runner loops to process new work
- **THEN** the configuration is reloaded from disk before processing

#### Scenario: Invalid config reload falls back
- **WHEN** the config file is invalid on reload
- **THEN** the previous valid configuration is used
- **AND** an error is logged
