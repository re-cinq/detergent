# Assembly Line

Deterministic agent invocation. Define a line of agent calls that will get invoked, in sequence, on every commit. Get alerted via the Claude Code statusline when downstream agents make changes, and use the `/line-rebase` skill to automatically pull them in.

Kinda like CI, but local.

Everything is in Git, so you lose nothing. If you _also_ use [`claudit`](https://github.com/re-cinq/claudit), your agent can automatically attach chat history as Git Notes.

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/re-cinq/assembly-line/master/scripts/install.sh | bash
```

Then, in your repo:

```bash
line init # Set up skills
line run # Start the daemon (there's a skill for this too)
line status # See what's going on
line status -f -n 1 # Follow the status, refreshing every 1 second
```

## Quick Start

Create a config file `line.yaml`:

```yaml
agent:
  command: claude
  args: ["--dangerously-skip-permissions", "-p"]

settings:
  watches: main

stations:
  - name: security
    prompt: "Review for security vulnerabilities. Fix any issues found."

  - name: docs
    prompt: "Ensure public functions have clear documentation."
    args: ["--dangerously-skip-permissions", "--model", "haiku", "-p"]

  - name: style
    prompt: "Fix any code style issues."
```

Stations are processed as an ordered line: each station watches the one before it, and the first station watches the branch specified in `settings.watches` (defaults to `main`). Individual stations can override the global `command` and `args` to use a different agent or model (as shown with `docs` above).

**Note:** Assembly Line automatically prepends a [default preamble](internal/config/config.go#L68) to every station prompt. The preamble tells the agent to proceed without asking questions and not to run `git commit` (Assembly Line commits changes automatically when the agent exits). You can override it globally with `preamble`, or per-station with a `preamble` field on individual stations:

```yaml
# Global override (applies to all stations)
preamble: "You are a code review bot. Proceed without asking questions."

stations:
  - name: security
    prompt: "Review for security vulnerabilities."
    # Per-station override (takes priority over global)
    preamble: "You are a security specialist. Be thorough and cautious."
```

### Permissions

If your agent is Claude Code, you can pre-approve tool permissions instead of using `--dangerously-skip-permissions`. Add an optional `permissions` block — line writes it as `.claude/settings.json` in each worktree before invoking the agent:

```yaml
permissions:
  allow:
    - Edit
    - Write
    - "Bash(*)"
```

## Why?

Models will absolutely forget things, especially if context is overloaded (there's too much) or polluted (too many different topics). However, if you prompt them with a clear context, they'll spot what they overlooked straight away.

### Why not...

* **...do it yourself?** As a human, trying to remember to run the same set of quality-check prompts before every commit is a hassle.
* **...do it in CI?** Leaving these tasks until CI delays feedback, your agent might not be configured to read from CI, and sometimes you don't want to push.
* **...use a Git hook?** Not being able to commit in a hurry would be inconvenient. Plus, with `line` you can tell your main agent to commit with `skip line` if you want.

## Usage

```bash
# Validate your config (defaults to line.yaml)
line validate

# See the station line
line viz

# Run once and exit
line run --once

# Run as daemon (polls for changes)
line run

# Check status of each station
line status

# Live-updating status (like watch, tails active agent logs)
line status -f

# View agent logs for a station
line logs security

# Follow agent logs in real-time
line logs -f security

# Use a different config file
line run --path my-config.yaml

# Initialize Claude Code integration (statusline + skills)
line init
```

## How It Works

1. Assembly Line watches branches for new commits
2. When a commit arrives, it creates a worktree for each triggered station
3. The agent receives: the prompt + upstream commit messages + diffs
4. Agent changes are committed with `[STATION]` tags and `Triggered-By:` trailers (pre-commit hooks are skipped — no agent is present after the runner exits)
5. If no changes needed, a git note records the review
6. Downstream stations see upstream commits and can build on them
7. The statusline shows `✓` next to stations that are up to date — use `/line-rebase` to pull them back into your working branch

### Getting changes back

Agent work accumulates on station branches (`line/security`, `line/style`, etc.). The `/line-rebase` skill merges the terminal station's branch back into main:

1. Finds the terminal station (the end of the line — nothing watches it)
2. Verifies the line is complete (no stations still running or failed)
3. Creates a backup branch (`pre-rebase-backup`) and stashes uncommitted work
4. Rebases main onto the terminal branch, resolving conflicts if needed
5. Restores stash and reports what happened

If anything goes wrong: `git reset --hard pre-rebase-backup`

**Note:** When running as a daemon, line automatically reloads `line.yaml` at the start of each poll cycle. Config changes take effect immediately without requiring a restart.

**Resilience:** On startup, line checks for and auto-repairs `core.bare=true` git config corruption (a known VS Code / concurrent-write race condition). If detected, it silently repairs the config so commands continue to work without manual intervention.

## Claude Code Integration

`line init` sets up:

- **Statusline** — shows the station pipeline in Claude Code's status bar:
  ```
  main ─── security ✓ ── docs ⟳ ── style ·
  ```
  - When on a terminal station branch that's behind HEAD, displays a bold yellow warning: `⚠ use /line-rebase to pick up latest changes`
- **Skills** — adds `/line-start` to start the daemon as a background task and `/line-rebase` for rebasing station branch changes onto their upstream

### Statusline Symbols

| Symbol | Meaning |
|--------|---------|
| `◎` | Change detected |
| `⟳` | Agent running / committing |
| `◯` | Pending (behind HEAD) |
| `✗` | Failed |
| `⊘` | Skipped |
| `✓` | Done (up to date) |
| `·` | Never run |

## Git Conventions

- **Branches**: `line/{station-name}` (configurable prefix)
- **Commits**: `[SECURITY] Fix SQL injection in login` with `Triggered-By: abc123` trailer
- **Notes**: `[SECURITY] Reviewed, no changes needed` when agent makes no changes
- **Agent detection**: Runner commits are identified solely by the `Triggered-By:` trailer. `Co-Authored-By:` lines from AI coding tools (Claude Code, Copilot, Cursor) are ignored — those commits are processed normally by the station line
- **Skipping processing**: Add `[skip ci]`, `[ci skip]`, `[skip line]`, or `[line skip]` to commit messages to prevent line from processing them

## Development

```bash
make build    # Build binary (bin/line); auto-codesigns on macOS
make install  # Install binary to $(go env GOBIN) or ~/go/bin
make test     # Run acceptance tests
make lint     # Run linter (requires golangci-lint)
make fmt      # Format code
```

## License

[AI Native Application License (AINAL) v2.0](LICENSE) ([source](https://github.com/re-cinq/ai-native-application-license))
