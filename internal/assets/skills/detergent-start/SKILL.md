---
name: detergent-start
description: Start the detergent daemon as a background task. It watches for commits and processes them through the concern chain.
metadata:
  author: detergent
  version: "1.0"
---

Start the detergent daemon as a Claude Code background task.

---

## Steps

1. **Find the config file**
   Look for `detergent.yaml` or `detergent.yml` starting from the repo root:
   ```bash
   git rev-parse --show-toplevel
   ```
   Check that directory for the config file. If not found, STOP: "No detergent.yaml found."

2. **Check if already running**
   ```bash
   pgrep -f "detergent run" || true
   ```
   If a process is found, tell the user: "Detergent daemon is already running (PID <pid>)." and ask if they want to restart it. If yes, kill the existing process first:
   ```bash
   pkill -f "detergent run"
   ```

3. **Start the daemon**
   Run using the Bash tool with `run_in_background: true`:
   ```bash
   detergent run <config-file>
   ```
   This starts detergent in continuous polling mode. It will process commits through the concern chain as they arrive.

4. **Confirm**
   Tell the user:
   ```
   Detergent daemon started. The statusline will update as concerns process.
   ```

---

## Guardrails

- **ALWAYS** use `run_in_background: true` for the Bash tool call. The daemon runs continuously.
- **ALWAYS** check for an existing daemon before starting a new one.
- Do not use `--once` mode — the point is continuous watching.
- If the daemon exits immediately, read the output and report the error.
