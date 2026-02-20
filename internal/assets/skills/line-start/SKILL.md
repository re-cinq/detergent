---
name: line-start
description: Start the line daemon as a background task. It watches for commits and processes them through the concern chain.
metadata:
  author: line
  version: "1.0"
---

Start the line daemon as a Claude Code background task.

---

## Steps

1. **Find the config file**
   Look for `line.yaml` or `line.yml` starting from the repo root:
   ```bash
   git rev-parse --show-toplevel
   ```
   Check that directory for the config file. If not found, STOP: "No line.yaml found."

2. **Check if already running**
   ```bash
   pgrep -f "line run" || true
   ```
   If a process is found, tell the user: "Assembly Line daemon is already running (PID <pid>)." and ask if they want to restart it. If yes, kill the existing process first:
   ```bash
   pkill -f "line run"
   ```

3. **Start the daemon**
   Run using the Bash tool with `run_in_background: true`:
   ```bash
   line run
   ```
   If the config file is not at the default `line.yaml`, use `--path`:
   ```bash
   line run --path /path/to/config.yaml
   ```
   This starts line in continuous polling mode. It will process commits through the concern chain as they arrive.

4. **Confirm**
   Tell the user:
   ```
   Assembly Line daemon started. The statusline will update as concerns process.
   ```

---

## Guardrails

- **ALWAYS** use `run_in_background: true` for the Bash tool call. The daemon runs continuously.
- **ALWAYS** check for an existing daemon before starting a new one.
- Do not use `--once` mode — the point is continuous watching.
- If the daemon exits immediately, read the output and report the error.
