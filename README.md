# Detergent

A daemon that orchestrates AI coding agents through a pipeline of concerns.

Instead of cramming security, documentation, and style checks into one prompt, Detergent runs specialized agents in sequence. Each agent sees the work of those before it, preserving intent as changes flow through the pipeline.

## Installation

### Quick install (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/re-cinq/detergent/master/scripts/install.sh | bash
```

The install script automatically:
- Downloads the latest release binary for your platform (macOS, Linux, Windows)
- Falls back to `go install` if release download fails
- Falls back to building from source if needed
- Signs binaries on macOS for Gatekeeper compatibility

### From source

```bash
go install github.com/re-cinq/detergent/cmd/detergent@latest
```

## Quick Start

Create a config file `detergent.yaml`:

```yaml
agent:
  command: claude
  args: ["-p"]

settings:
  poll_interval: 30s
  branch_prefix: detergent/

concerns:
  - name: security
    watches: main
    prompt: "Review for security vulnerabilities. Fix any issues found."

  - name: docs
    watches: security
    prompt: "Ensure public functions have clear documentation."

  - name: style
    watches: main
    prompt: "Fix any code style issues."
```

This creates a graph where:
- `security` and `style` watch `main` (run in parallel)
- `docs` watches `security` (runs after security completes)

### Permissions

If your agent is Claude Code, you can pre-approve tool permissions instead of using `--dangerously-skip-permissions`. Add an optional `permissions` block — detergent writes it as `.claude/settings.json` in each worktree before invoking the agent:

```yaml
permissions:
  allow:
    - Edit
    - Write
    - "Bash(*)"
```

## Usage

```bash
# Validate your config (defaults to detergent.yaml)
detergent validate

# See the concern graph
detergent viz

# Run once and exit
detergent run --once

# Run as daemon (polls for changes)
detergent run

# Check status of each concern
detergent status

# Live-updating status (like watch, tails active agent logs)
detergent status -f

# View agent logs for a concern
detergent logs security

# Follow agent logs in real-time
detergent logs -f security

# Use a different config file
detergent run --path my-config.yaml

# Install Claude Code integration (statusline + skills)
detergent install
```

## How It Works

1. Detergent watches branches for new commits
2. When a commit arrives, it creates a worktree for each triggered concern
3. The agent receives: the prompt + upstream commit messages + diffs
4. Agent changes are committed with `[CONCERN]` tags and `Triggered-By:` trailers
5. If no changes needed, a git note records the review
6. Downstream concerns see upstream commits and can build on them

**Note:** When running as a daemon, detergent automatically reloads `detergent.yaml` at the start of each poll cycle. Config changes take effect immediately without requiring a restart.

## Claude Code Integration

`detergent install` sets up:

- **Statusline** — shows the concern pipeline in Claude Code's status bar:
  ```
  main ─── security ✓ ── docs ⟳ ── style ·
  ```
  - When on a terminal concern branch that's behind HEAD, displays a bold yellow warning: `⚠ use /rebase <branch> to pick up latest changes`
- **Skills** — adds `/detergent-start` to start the daemon as a background task and `/rebase` for rebasing concern branch changes onto their upstream

### Statusline Symbols

| Symbol | Meaning |
|--------|---------|
| `◎` | Change detected |
| `⟳` | Agent running / committing |
| `◯` | Pending (behind HEAD) |
| `✗` | Failed |
| `⊘` | Skipped |
| `*` | Done, produced modifications |
| `✓` | Done, no changes needed |
| `·` | Never run |

## Git Conventions

- **Branches**: `detergent/{concern-name}` (configurable prefix)
- **Commits**: `[SECURITY] Fix SQL injection in login` with `Triggered-By: abc123` trailer
- **Notes**: `[SECURITY] Reviewed, no changes needed` when agent makes no changes
- **Skipping processing**: Add `[skip ci]`, `[ci skip]`, `[skip detergent]`, or `[detergent skip]` to commit messages to prevent detergent from processing them

## Development

```bash
make build    # Build binary (bin/detergent)
make test     # Run acceptance tests
make lint     # Run linter (requires golangci-lint)
make fmt      # Format code
```

## License

[AI Native Application License (AINAL) v2.0](LICENSE) ([source](https://github.com/re-cinq/ai-native-application-license))
