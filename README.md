# Assembly Line

A tool for running tasks on and after commit when working with Claude Code. It builds a CLI called `line`. The intention is to run local deterministic tools like linters as pre-commit hooks (called Gates), and then post-commit run Claude Code (or another command) against the new change (these are Stations). Stations run in an ordered list, and are implemented using Git branches.

Rather than expecting an agent to remember to do various checks, a user can 'hard code' a set of prompts to run against every commit. These might include DRYing out code, checking test coverage, updating docs.

## Configuration

The tool is configured with YAML. All commands assume the config file is `line.yaml`; commands that need to reference the config accept `-p`/`--path` to specify a different path. A Git branch to watch must be configured (`watches`).

```yaml
agent:
  command: claude
  args: ["--dangerously-skip-permissions", "--model", "sonnet", "-p"]

settings:
  watches: master

gates:
  - name: lint
    run: "golangci-lint run ./..."

stations:
  - name: deadcode
    prompt: "Remove any unused code."
  - name: dry
    args: ["--dangerously-skip-permissions", "--model", "haiku", "-p"]
    prompt: "Deduplicate code."
  - name: test
    args: ["--dangerously-skip-permissions", "--model", "haiku", "-p"]
    prompt: "Run all the tests, fix any failures."
  - name: docs
    prompt: "Ensure README is up to date with latest features."
```

### Gates

An ordered list of Gates can be configured — each runs as a Git pre-commit hook.

### Stations

- A default agent `command` and `args` can be configured and are shared by all stations.
- Each station can override the agent `command` and/or `args`.
- Each station can be configured with a `prompt`.
- Station names must be unique; each maps to a Git branch (`line/stn/<name>`).

### Settings

- `watches` (required): Git branch to watch.
- `auto_rebase` (bool, default `false`): Enable automatic rebase when the terminal station has unpicked commits. Requires PostToolUse and Stop hooks (installed by `line init`).

## Commands

### `line init`

- Appends a Git pre-commit hook invoking `line gate`.
- Preserves any existing Git pre-commit hooks.
- Appends a Git post-commit hook invoking `line run`.
- Converges on the desired state — re-running is safe; old or out-of-date config is updated.
- Installs the `/line-rebase` and `/line-preview` skills.
- Configures Claude Code to use `line statusline` for its statusline.
- Adds `.gitignore` entries for any temporary files introduced by assembly-line.
- Installs PostToolUse and Stop hooks running `line auto-rebase-hook`.

### `line remove`

- Removes the assembly-line blocks from pre-commit and post-commit Git hooks, preserving any other hook content.
- Removes the `/line-rebase` and `/line-preview` skill directories.
- Removes the `statusLine` key from `.claude/settings.json`, preserving other settings.
- Removes the PostToolUse and Stop hook entries from `.claude/settings.json`, preserving other hooks.
- Removes the assembly-line block from `.gitignore`, preserving other entries.
- Safe to run even when assembly-line was never initialized (no-op).

### `line run`

- Each station is executed in sequence.
- Each station operates on its own branch; stations must not operate on any other branches.
- Stations must not re-trigger `line run`.
- A default preamble prompt is prepended to each station's configured prompt, instructing the agent not to commit.
- Stations commit any changes made by the invoked agent/command on its branch.
- Stations run in isolated ephemeral Git worktrees under the system temp dir, so the user can keep working in their repo while the line runs.
- Stations 'just work' — if Git state is bad, they catch up to the watched branch and resume.
- Changes to files listed in `.lineignore` (gitignore syntax) do not trigger the line.
- Commits containing `[skip ci]`, `[ci skip]`, `[skip line]`, or `[line skip]` in the message do not trigger the line.
- Line runs are independent of rebases on the watched branch.
- If a new commit arrives while the line is running, all agents are stopped, existing station-branch commits are preserved, and the line restarts from the beginning with the latest commit.
- Stations rebase onto their predecessor (not merge) to keep history linear.
- A failed station blocks the line and is reported as 'failed'.

### `line clear`

- Stops any active line runs, terminates all agents, clears all state files, drops the station branches and worktrees.
- Prompts for confirmation unless `--force` is passed.

### `line status`

- Prints a headed list of all stations, starting with the watched branch. For each station the shortref of HEAD is shown, along with a dirty-directory indicator.
- Printed before the station list: `⏸` (grey) for an inactive line or `▶` (green) for an active line runner, followed by the config file name.
- Per-station symbols and colour-coded states:
  - ✓ **up to date** — the only commits between the station and the watched branch HEAD are skip-marker commits (green)
  - ● **agent running** — an agent is currently running; shows uptime duration (e.g. `52s`, `5m 32s`) (orange)
  - ○ **pending** — no agent running and station has not yet processed the latest commit (yellow)
  - ✗ **failed** — station encountered an error (red)
- A commit-distance indicator is shown between each station name and its HEAD ref: `H` marks HEAD; each `+` after `H` is one commit the station is ahead; each `-` before `H` is one commit behind.
- `line status -f` refreshes every two seconds, flicker-free with a hidden cursor.
- Status is computed on-demand rather than cached, so it is trustworthy and reliable.

### `line statusline`

- Shows the same state as `line status` in a single-line format for Claude Code's statusline.
- When the terminal station has commits not yet in the watched branch, prompts the user to use the `/line-rebase` skill to pick them up.
- Provided by the `statusline` subcommand with no external dependencies.

### `/line-rebase` Skill

- Safely stashes any current work on the watched branch, rebases from the terminal station branch to pick up the latest changes, then unstashes work in progress. No work is ever lost.
- Commits picked up onto the watched branch are marked so they do not re-trigger the line.

### `/line-preview` Skill

- Shows a read-only summary of unpicked changes: what each station actually changed (content diffs), not commit history. All derived from Git on-demand with no state files.

### `line rebase`

- Deterministic stash → rebase → unstash from the terminal station branch onto the watched branch. Must be run from the watched branch; refuses otherwise.
- On conflict: aborts the rebase, restores the stash, and reports failure. Never auto-resolves conflicts.
- Reports the list of changed files after a successful rebase.

### `line auto-rebase-hook`

- PostToolUse and Stop hook. When `settings.auto_rebase: true` and the terminal station has unpicked commits, performs a rebase and reports changed files.
- Deduplicates attempts — does not re-attempt for the same terminal ref.
- Exits silently when: no config, `auto_rebase` is false, no stations, no unpicked commits, already attempted for the current ref, or a line run is in progress.
- `line clear` removes the dedup marker.

### `line schema`

Outputs the YAML configuration schema, intended to help coding agents write valid config.

### `line validate`

Validates `line.yaml` and outputs specific, helpful error messages if the config is invalid. Intended for use by coding agents.

### `line explain`

Outputs succinct but complete usage information about the tool — its purpose, commands, and config — for the benefit of coding agents. Like this README, but always available via CLI.
