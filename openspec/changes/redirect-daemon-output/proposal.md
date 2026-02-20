## Why

When running in daemon mode, agent output streams directly to the terminal. This creates confusion when working in that terminal - output from multiple concerns intermixes with your work, making it hard to distinguish daemon activity from your own commands.

## What Changes

- Agent stdout/stderr redirected to log files instead of the terminal
- Log files created in system temp directory, named by concern (e.g., `/tmp/line-security.log`)
- Each agent invocation prefixed with the commit hash being processed, providing context for log readers
- Daemon status messages (startup, shutdown, poll errors) remain on terminal for immediate feedback

## Capabilities

### New Capabilities
- `concern-logging`: Specifies how agent output is captured and stored, including log file location, naming convention, and per-invocation context headers

### Modified Capabilities
None - this is additive behavior that doesn't change existing requirements

## Impact

- `internal/engine/engine.go`: Change agent command output from `os.Stdout`/`os.Stderr` to file handles
- Log file lifecycle: created on first agent run, appended thereafter
- Users will need to `tail -f` log files to watch agent activity (could document in status output)
