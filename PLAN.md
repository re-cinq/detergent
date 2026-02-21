# Assembly Line: Requirements Specification

## Purpose

A daemon that orchestrates coding agents in a station-based pipeline. Each agent focuses on a single quality station (security, deduplication, style, etc.). Changes flow through a directed graph of stations, with Git providing the audit trail and intent preservation.

---

## Core Concepts

### Station
A single-purpose agent focus. Examples:
- "Ensure this code has no security vulnerabilities"
- "Remove code duplication"
- "Ensure public APIs are documented"

### Station Graph
Stations are arranged in a directed acyclic graph (DAG). Each station:
- Watches exactly one upstream branch (either a source branch like `main`, or another station's output branch)
- Pushes to its own output branch
- Has no knowledge of what watches it downstream

### Intent Preservation
When an agent runs, it receives:
- The diffs from upstream commits (what changed)
- The commit messages from upstream commits (why it changed)
- Its own station prompt (what to focus on)

This allows agents to respect work done by earlier stations.

### Implicit Priority
Position in the graph implies priority. Earlier stations take precedence. A downstream agent should not undo upstream work unless it can preserve the original intent.

---

## Configuration

The system is configured via a YAML file that defines:

### Repository Settings
- Path to the repository (or current directory)
- Branch prefix for station output branches (e.g., `line/`)
- Poll interval for watching branches

### Station Definitions
Each station specifies:
- **name**: Identifier for the station (used in branch names and commit tags)
- **watches**: The branch this station monitors (a source branch or another station's name)
- **prompt**: The task description given to the agent

### Example Configuration

```yaml
repository: .
branch_prefix: line

stations:
  - name: deduplication
    watches: main
    prompt: |
      Review this code for duplication. Consolidate repeated logic into
      shared functions where it improves maintainability.

  - name: security
    watches: deduplication
    prompt: |
      Review this code for security vulnerabilities. Fix any issues found.
      Respect changes made by previous agents (see commit history below).

  - name: documentation
    watches: security
    prompt: |
      Ensure public APIs have clear documentation.

```

---

## Git Conventions

### Branch Naming
- Station output branches are named: `{branch_prefix}/{station_name}`
- Example: station "security" → branch `line/security`

### Commit Message Format
```
[{STATION_NAME}] {summary}

{description of changes and reasoning}

Triggered-By: {upstream_commit_hash}
```

The `[STATION_NAME]` tag allows downstream agents (and humans) to identify which station made each change.

### Git Notes
When a station reviews commits but makes no changes, it records this via git notes:
```
git notes add -m "[{STATION_NAME}] Reviewed, no changes needed" {commit_hash}
```

View the audit trail with:
```
git log --show-notes
```

### Worktrees
Each station operates in its own git worktree. This allows:
- Parallel execution of independent stations
- Clean isolation between station states
- No interference from in-progress work

---

## Daemon Behavior

### Startup
1. Parse configuration
2. Validate the station graph (check for cycles, missing references)
3. For each station, create a worktree if one doesn't exist
4. Initialize branch tracking state (last-seen commit per watched branch)

### Main Loop
For each station, continuously:
1. Check watched branch for new commits since last seen
2. If new commits exist:
   - Assemble context (see below)
   - Invoke the agent with the context
   - If changes were made, commit and push to output branch
   - If no changes were made, add git notes (output branch already advanced via pre-run rebase)
   - Update last-seen commit
3. Sleep for poll interval

### Context Assembly
When triggering an agent, provide:

```
## Recent Changes from Upstream

### Commit: {hash} [{STATION_TAG}]
{commit message}

```diff
{diff content}
```

[...repeat for each new commit...]

---

## Your Station

{station prompt from config}

---

## Instructions

- The code is checked out at the state after the above commits
- Make changes that address your station
- Respect changes made by upstream agents unless you can preserve their intent
- Explain your reasoning in commit messages
```

### No-Change Handling
Before an agent runs, the station's output branch is rebased onto the watched branch. This replays any prior station commits on top of the latest upstream state, handling both the clean (no prior commits) and diverged (prior commits exist) cases.

If an agent makes no changes after the rebase:
- The output branch is already at or ahead of upstream (rebase already advanced it)
- Add a git note to each processed commit recording the review: `[{STATION_NAME}] Reviewed, no changes needed`
- Continue watching for future changes

This ensures:
- Downstream stations are triggered (they watch the output branch, which has now advanced)
- Audit trail is preserved (git notes record that the station reviewed the code)
- History stays clean (original commits pass through, station commits are replayed on top)

### Error Handling
If an agent fails:
- Log the error
- Do not update last-seen commit (retry on next poll)
- Continue processing other stations

---

## Branch Lifecycle

### First Run
When a station runs for the first time and its output branch doesn't exist:
1. Create the branch from the current state of the watched branch
2. Create the worktree for this branch

### Ongoing Operation
- The output branch advances via rebase onto the watched branch before each agent run, then optionally via agent commits
- Downstream stations see these commits and react
- Git notes on commits provide audit trail of which stations reviewed them

### Branch Reset (Manual)
If a user wants to "restart" a station:
- Delete the station's branch
- On next poll, it will be recreated from upstream

---

## Observability

The daemon should provide:
- Logging of when each station is triggered
- Logging of agent invocations and outcomes
- Current state inspection (which commit each station last processed)

### Graph Visualization (Terminal)

A `line viz` command that shows the configured station graph in the terminal (see below).

A `line status` command that displays the station graph in the terminal:

```
$ line status

main
 │
 ├─→ [deduplication] ✓ caught up (abc123)
 │    │
 │    └─→ [security] ⟳ processing... (abc123)
 │         │
 │         └─→ [documentation] ◯ waiting (def456)
 │
 └─→ [linting] ✓ caught up (abc123)
```

Status indicators:
- `✓` caught up with upstream
- `⟳` currently processing
- `◯` waiting (upstream hasn't advanced yet)
- `✗` last run failed (will retry)

---

## Future Considerations (Not in Initial Scope)

- **Squash option**: Optionally squash commits at the end of the line - maybe a preconfigured station?
- **Configurable agents**: Support agents other than Claude Code
- **Conflict detection**: Alert when an agent reverts upstream work
- **Human checkpoints**: Pause for review before certain stations run
- **Parallel station execution**: Run independent graph branches concurrently
- **Containerisation**: Ensure agents work safely within containers, and so they can run without permissions
- **Web dashboard**: Animated web view showing commits flowing through the station graph in real-time, with drill-down into diffs and agent reasoning

---

## Verification Criteria

The implementation is correct when:

1. **Graph execution**: Changes to the source branch propagate through all downstream stations
2. **Context passing**: Each agent receives commit messages and diffs from upstream
3. **Commit tagging**: All agent commits include the station tag in the message
4. **Audit trail**: Git notes record when stations review commits without changes
5. **Isolation**: Stations operate in separate worktrees without interference
6. **Idempotency**: Running the daemon when nothing changed produces no new commits
7. **Recovery**: After an error, the next poll retries the failed work
