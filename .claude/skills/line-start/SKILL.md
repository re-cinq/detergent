---
name: line-start
description: Start the line runner to process commits through the station line. Normally auto-triggered by post-commit hooks.
metadata:
  author: line
  version: "2.0"
---

Start the line runner. Normally this happens automatically via the post-commit hook installed by `line init`, but you can run it manually if needed.

---

## How it works

After `line init`, a post-commit hook calls `line trigger` on every commit. This:
1. Writes a trigger file (`.line/trigger`) with the current HEAD
2. Spawns `line run` if no runner is already active

The runner processes all pending commits through the station line, then waits a grace period for more work. If no new commits arrive, it exits on its own. No manual start/stop needed.

## Manual start

If the runner isn't starting automatically (e.g., hook not installed), run it manually:

1. **Find the config file**
   Look for `line.yaml` or `line.yml` starting from the repo root:
   ```bash
   git rev-parse --show-toplevel
   ```

2. **Start the runner**
   Run using the Bash tool with `run_in_background: true`:
   ```bash
   line run
   ```
   If the config file is not at the default `line.yaml`, use `--path`:
   ```bash
   line run --path /path/to/config.yaml
   ```
   The runner will process pending commits and exit when there's no more work.

3. **Confirm**
   Tell the user:
   ```
   Assembly Line runner started. It will process pending commits and exit when done.
   ```

## Debugging

For one-shot processing (no grace period wait):
```bash
line run --once
```

---

## Guardrails

- The runner self-retires after processing — no need to kill it.
- If you need continuous background processing, use `run_in_background: true` for the Bash tool call.
- If the runner exits immediately saying "already active", another instance is running.
- Do not use `--once` mode for normal operation — it skips the grace period and may miss rapid commits.
