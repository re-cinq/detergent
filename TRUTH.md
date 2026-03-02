# Assembly Line

A tool for running tasks on and after commit when working with Claude Code. It builds a CLI called `line`. The intention is to run local deterministic tools like linters as pre-commit hooks (called Gates), and then post-commit run Claude Code (or another command) against the new change (these are Stations). Stations run in an ordered list, and are implemented using Git branches.

The intention is that rather than expecting an agent to remember to do various checks, a user can 'hard code' a set of prompts to run against a commit. These might include DRYing out code, checking test coverage, updating docs.

## Config

- **CFG-1**: The tool is configured with YAML. All commands assume this is `line.yaml`; if they need reference to the config they take a `-p` or `--path` command.
- **CFG-2**: A Git branch to watch must be configured (`watches`).

- Example:

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
        prompt: "If you use beads, act on the work immediately and do not exit until those beads have been resolved. Remove any unused code. Do not change any other files and do not push. Don't bother testing, we'll do that later."
      - name: dry
        args: ["--dangerously-skip-permissions", "--model", "haiku", "-p"]
        prompt: "If you use beads, act on the work immediately and do not exit until those beads have been resolved. Deduplicate code. Do not change any other files and do not push. Don't bother testing, we'll do that later."
      - name: test
        args: ["--dangerously-skip-permissions", "--model", "haiku", "-p"]
        prompt: "If you use beads, act on the work immediately and do not exit until those beads have been resolved. Run all the tests, fix any failures. Do not reduce test coverage. Do not change any other files and do not push."
      - name: docs
        prompt: "If you use beads, act on the work immediately and do not exit until those beads have been resolved. Ensure README is up to date with latest features. Don't change any other files and do not push."
    ```

### Gates

- **CFG-GATE-1**: An ordered list of Gates can be configured - each of these is a Git pre-commit hook.

### Stations

- **CFG-STN-1**: A default agent command `agent` can be configured for all Stations.
- **CFG-STN-2**: A default array of arguments `args` can be configured for all Stations.
- **CFG-STN-3**: Each Station can be configured with a custom agent command.
- **CFG-STN-4**: Each Station can be configured with custom argument array.
- **CFG-STN-5**: Each Station can be configured with a prompt `prompt`.

## Behaviour

### `line init`

- **INIT-1**: Appends a Git pre-commit hook in the CWD, invoking `line gate`
- **INIT-2**: Preserves any existing Git pre-commit hooks.
- **INIT-3**: Appends a Git post-commit hook in the CWD, invoking `line run`
- **INIT-4**: Converges on the desired state - so if old or out-of-date config exists, it should be updated.
- **INIT-5**: Installs the `/line-rebase` and `/line-preview` skills.
- **INIT-6**: Configures Claude Code to use `line statusline` for its statusline.
- **INIT-7**: Adds `.gitignore` entries for any temporary files introduced by assembly-line.

### `line remove`

- **RMV-1**: Removes the assembly-line blocks from pre-commit and post-commit Git hooks, preserving any other hook content.
- **RMV-2**: Removes the `/line-rebase` and `/line-preview` skill directories.
- **RMV-3**: Removes the `statusLine` key from `.claude/settings.json`, preserving other settings.
- **RMV-4**: Removes the assembly-line block from `.gitignore`, preserving other entries.
- **RMV-5**: Safe to run when assembly-line was never initialized (no-op).

### `line run`

- **RUN-1**: Each station is executed in sequence.
- **RUN-2**: Each station operates on its own branch.
- **RUN-3**: Stations must not operate on any other branches.
- **RUN-4**: Stations must not re-trigger `line run`.
- **RUN-5**: Stations should commit any changes made by the invoked agent/command on its branch.
- **RUN-6**: Stations should 'just work' - if all else fails due to Git state, they should 'catch up' to their watched branch and resume from there.
- **RUN-7**: Changes to files listed in `.lineignore` should not trigger a line.
- **RUN-8**: `.lineignore` should be configured exactly as `.gitignore`.
- **RUN-9**: The line should not be triggered for commits containing these markers in the message: [skip ci], [ci skip], [skip line], [line skip]
- **RUN-10**: Line runs should be independent of rebases on the watched branch.
- **RUN-11**: If a new run is started while one is in progress, any commits on station branches are preserved. All agents are stopped in the previous run, and the line starts again from the beginning, taking the latest commit from the watched branch.
- **RUN-12**: Each Station should have a default preamble prompt prepended to its configured prompt, instructing the agent that it must not commit.
- **RUN-13**: A station must be able to invoke Claude Code in non-interactive mode (`-p`) and have it make real file changes on the station branch.
- **RUN-14**: A failed station must block the line and be reported as 'failed'.
- **RUN-15**: The user must be able to continue working in their repo while a line is running: all stations must operate in ephemeral git worktrees under the system temp dir.
- **RUN-16**: Stations must rebase onto their predecessor, not merge, to keep history linear.

### `line clear`

- **CLEAR-1**: Stops any active line runs, terminates all agents, clears all state files, drops the station branches and worktrees.
- **CLEAR-2**: Prompts for confirmation unless `--force` is passed.

### `line status`

- **STAT-1**: Prints a list of all stations, starting with the watched branch. For each station the shortref of HEAD is shown, along with an indicator of if the dir is dirty.
- **STAT-2**: When an agent is running for a station, it is marked as "agent running". When a station has 'seen' and acted on a commit, is marked as "up to date". If no agent is running and a station has not yet processed an update, it is marked "pending". A running agent also shows the PID and start time. Statuses should be colour-coded: green for up-to-date, orange for in progress, yellow for pending, red for failed.
- **STAT-3**: Printed before the list is the grey symbol `⏸` for an inactive line, or `▶` in green for an active line runner. Following is the name of the config file used (eg `▶ line.yaml`)
- **STAT-4**: `line status -f` refreshes every two seconds, flicker-free with a hidden cursor.
- **STAT-5**: State must be, as much as possible, computed on-demand rather than cached in files. `line status` must be trustworthy and reliable.
- **STAT-6** Status should show headings, and to the left of each station one of the following symbols should be printed in the appropriate colour:
    - ✓ up-to-date
    - ✗ failed
    - ○ pending
    - ● in progress
- **STAT-7** An in-progress station should show how long the respective agent PID has been alive for (eg `52s`;`5m 32s`)
- **STAT-8**: A station is considered "up to date" if the only commits between its HEAD and the watched branch HEAD are skip-marker commits (`[skip line]`, `[line skip]`, `[skip ci]`, `[ci skip]`).

### `line statusline`

- **SL-1**: The Claude Code statusline should show the same state as `line status` in a one-line format.
- **SL-2**: When there are commits on the terminal station that are not in the source watched branch, the statusline should prompt the user to use the `/line-rebase` skill to pick them up.
- **SL-3**: This is provided by the `statusline` subcommand, with no external dependencies.

### Skill

- **SKL-1**: `/line-rebase` should, perfectly safely, stash any current work on the watched branch, rebase from the terminal branch to pick up latest changes, and unstash work in progress. It is vital that no work ever be lost, and this be fast and automatic.
- **SKL-2**: When changes are picked onto the main branch, these commits must not trigger the line again.
- **SKL-3**: `/line-preview` should show a read-only summary of unpicked
  changes: what each station actually changed (content diffs), not commit
  history. All derived from Git on-demand with no state files.

### `line schema`

- **SCH-1**: Outputs the YAML configuration schema, with the intention of teaching coding agents how to write config.

### `line validate`

- **VAL-1**: Validates YAML configuration, outputting specific, helpful error messages if the config is invalid. Intended for use by coding agents.

### `line explain`

- **EXP-1**: Outputs succinct but complete usage information about the tool, its purpose, commands and config, for the benefit of coding agents. Like a README, but for agents, and always available.
