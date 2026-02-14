# Detergent

A daemon that orchestrates AI coding agents through a pipeline of concerns.

Instead of cramming security, documentation, and style checks into one prompt, Detergent runs specialized agents in sequence. Each agent sees the work of those before it, preserving intent as changes flow through the pipeline.

## Installation

```bash
go install ./cmd/detergent
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

## Usage

```bash
# Validate your config
detergent validate detergent.yaml

# See the concern graph
detergent viz detergent.yaml

# Run once and exit
detergent run --once detergent.yaml

# Run as daemon (polls for changes)
detergent run detergent.yaml

# Check status of each concern
detergent status detergent.yaml

# Live-updating status (like watch)
detergent status -f detergent.yaml

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

## Claude Code Integration

`detergent install` sets up:

- **Statusline** — shows the concern pipeline in Claude Code's status bar:
  ```
  main ─── security ✓ ── docs ⟳ ── style ·
  ```
- **Skills** — adds `/rebase` for landing concern branch changes

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

## License

MIT
