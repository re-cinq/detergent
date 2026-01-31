## ADDED Requirements

### Requirement: Agent output redirected to log files
When running in daemon mode, the system SHALL redirect agent stdout and stderr to dedicated log files instead of the terminal. Each concern SHALL have its own log file.

#### Scenario: Agent output goes to log file
- **WHEN** an agent runs for a concern
- **THEN** the agent's stdout and stderr are written to a log file, not the terminal

#### Scenario: Separate log file per concern
- **WHEN** agents run for concerns "security" and "style"
- **THEN** output appears in separate files: one for security, one for style

### Requirement: Log files located in system temp directory
The system SHALL create log files in the system temporary directory with the naming pattern `detergent-<concern>.log`.

#### Scenario: Log file naming
- **WHEN** an agent runs for concern "security"
- **THEN** output is written to `<temp-dir>/detergent-security.log`

#### Scenario: Log file created on first agent run
- **WHEN** an agent runs for a concern for the first time
- **THEN** the log file is created if it does not exist

### Requirement: Log files opened in append mode
The system SHALL open log files in append mode, preserving output across daemon restarts and multiple agent invocations.

#### Scenario: Output preserved across invocations
- **WHEN** an agent runs, then another agent runs for the same concern
- **THEN** both outputs appear in the log file in sequence

#### Scenario: Output preserved across daemon restarts
- **WHEN** the daemon stops and starts again
- **THEN** new output appends to existing log content

### Requirement: Commit context header before each invocation
The system SHALL write a header line before each agent invocation containing the commit hash being processed and a timestamp.

#### Scenario: Header format
- **WHEN** processing commit abc1234
- **THEN** the log file contains a header like `--- Processing abc1234 at 2024-01-15T10:30:00Z ---` before the agent output

#### Scenario: Headers separate multiple invocations
- **WHEN** two commits are processed sequentially
- **THEN** each commit's output is preceded by its own header line

### Requirement: Daemon status messages remain on terminal
The system SHALL continue to write daemon lifecycle messages (startup, shutdown, poll errors) to the terminal for immediate operator feedback.

#### Scenario: Startup message on terminal
- **WHEN** the daemon starts
- **THEN** the startup message appears on the terminal, not in log files

#### Scenario: Agent output separate from daemon messages
- **WHEN** the daemon runs and an agent produces output
- **THEN** daemon messages appear on terminal and agent output appears in log files

### Requirement: Log file paths displayed at startup
The system SHALL display the log file path pattern at daemon startup so users know where to find agent output.

#### Scenario: Log path information at startup
- **WHEN** the daemon starts
- **THEN** the terminal shows the log file location pattern (e.g., "Agent logs: /tmp/detergent-<concern>.log")
