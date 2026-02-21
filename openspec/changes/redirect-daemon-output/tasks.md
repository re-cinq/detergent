## 1. Log File Management

- [x] 1.1 Add a map to store open log file handles per station in engine state
- [x] 1.2 Create helper function to get or create log file for a station (path: `<temp-dir>/line-<station>.log`, append mode)
- [x] 1.3 Add cleanup function to close all open log files on daemon shutdown

## 2. Agent Output Redirection

- [x] 2.1 Modify `runAgent` to accept a log file writer instead of using os.Stdout/os.Stderr
- [x] 2.2 Write commit context header to log file before running agent (`--- Processing <hash> at <timestamp> ---`)
- [x] 2.3 Set cmd.Stdout and cmd.Stderr to the station's log file

## 3. Daemon Startup Message

- [x] 3.1 Add log path pattern message to daemon startup output (e.g., "Agent logs: /tmp/line-<station>.log")

## 4. Testing

- [x] 4.1 Write acceptance test: agent output appears in log file, not terminal
- [x] 4.2 Write acceptance test: separate log files per station
- [x] 4.3 Write acceptance test: log file contains commit hash header before agent output
- [x] 4.4 Write acceptance test: daemon messages still appear on terminal
