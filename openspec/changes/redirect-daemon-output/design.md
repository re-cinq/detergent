## Context

Currently, when Detergent runs in daemon mode, agent output (stdout/stderr) streams directly to the terminal. This is the default behavior from `exec.Cmd` in Go. Users running the daemon in a terminal session see interleaved output from multiple concerns mixed with their own shell interactions, making it difficult to:

1. Work in the terminal while the daemon runs
2. Trace which output belongs to which concern
3. Review past agent activity after the fact

The engine currently creates commands via `exec.Command` and runs them without explicit output redirection, inheriting the daemon's stdout/stderr.

## Goals / Non-Goals

**Goals:**
- Redirect per-concern agent output to dedicated log files
- Provide clear context (commit hash) at the start of each agent invocation
- Keep daemon lifecycle messages (startup, shutdown, errors) visible on terminal
- Allow users to monitor agent activity via standard tools (tail -f)

**Non-Goals:**
- Log rotation or cleanup (users manage temp files per system policy)
- Structured logging format (plain text is sufficient for agent output)
- Configurable log paths (system temp directory is the sensible default)
- Real-time streaming to multiple destinations (file only, not file + terminal)

## Decisions

### 1. Log file location: System temp directory

**Decision:** Use `os.TempDir()` for log files (e.g., `/tmp/detergent-<concern>.log`)

**Rationale:**
- System temp is universally writable, no permissions issues
- Cleared on reboot, avoiding unbounded growth
- Familiar location for debugging artifacts
- No configuration required

**Alternatives considered:**
- Working directory: clutters project, may have write permission issues
- XDG data directory: more complex, overkill for ephemeral logs
- Configurable path: adds complexity without clear benefit

### 2. One log file per concern, append mode

**Decision:** Each concern gets a dedicated log file, opened in append mode. File created on first agent invocation for that concern.

**Rationale:**
- Keeps concern output separated and easy to tail individually
- Append mode preserves history across daemon restarts
- Lazy creation avoids empty files for concerns that never run

**Alternatives considered:**
- Single combined log: harder to filter by concern
- New file per invocation: many small files, harder to follow
- Truncate on daemon start: loses useful history

### 3. Commit hash prefix for each invocation

**Decision:** Before running an agent, write a header line to the log: `--- Processing <commit-hash> at <timestamp> ---`

**Rationale:**
- Provides context when reviewing logs
- Makes it easy to search for specific commit processing
- Timestamp helps correlate with git log

**Alternatives considered:**
- No prefix: harder to correlate output with commits
- JSON metadata: overkill, harder to read in tail output
- Separate metadata file: unnecessary complexity

### 4. File handle management

**Decision:** Open log files once per concern (on first use), keep handles open for daemon lifetime, close on shutdown.

**Rationale:**
- Avoids repeated open/close overhead
- Ensures writes are sequential
- Clean shutdown closes files properly

**Alternatives considered:**
- Open/close per invocation: more overhead, potential for interleaving
- Global file handle map: this is essentially what we'll use, stored in engine state

## Risks / Trade-offs

**[Risk] Log files grow unbounded during long daemon sessions**
→ Mitigation: System temp is typically cleaned on reboot. Users can truncate files or configure system-level temp cleanup. Document this behavior.

**[Risk] Concurrent writes if same concern runs twice**
→ Mitigation: The current engine design processes one commit per concern at a time. If this changes, we'd need per-file mutex. Accept this limitation for now.

**[Risk] Lost output if daemon crashes**
→ Mitigation: Go's `os.File` writes are typically flushed on each `Write` call to file. Accept that buffered output may be lost on hard crash.

**[Trade-off] Users must know to check log files**
→ Mitigation: Print log file paths at daemon startup. Could enhance status command later to show log locations.
