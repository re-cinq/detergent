# detergent Specification

## Purpose
TBD - created by archiving change add-detergent-daemon. Update Purpose after archive.
## Requirements
### Requirement: Concern Definition
The system SHALL allow users to define concerns via YAML configuration. Each concern MUST specify a name, watched branch, and prompt. The concern name MUST be used in branch naming and commit tagging.

#### Scenario: Valid concern configuration
- **WHEN** a concern is defined with name "security", watches "main", and a prompt
- **THEN** the system accepts the configuration
- **AND** creates output branch `{prefix}/security`

#### Scenario: Missing required field
- **WHEN** a concern is defined without a name, watches, or prompt
- **THEN** the system rejects the configuration with a clear error message

---

### Requirement: Concern Graph Validation
The system SHALL validate that concerns form a directed acyclic graph (DAG). The system MUST reject configurations with cycles or references to non-existent concerns.

#### Scenario: Valid DAG
- **WHEN** concerns form a valid DAG (e.g., main → dedup → security → docs)
- **THEN** the system accepts the configuration

#### Scenario: Cycle detection
- **WHEN** concerns form a cycle (e.g., A watches B, B watches A)
- **THEN** the system rejects the configuration with "cycle detected" error

#### Scenario: Missing reference
- **WHEN** a concern watches a non-existent concern
- **THEN** the system rejects the configuration with "unknown reference" error

---

### Requirement: Branch Watching
The system SHALL poll watched branches at a configurable interval. When new commits are detected, the system MUST trigger the corresponding concern's agent.

#### Scenario: New commits detected
- **WHEN** the watched branch has commits newer than last-seen
- **THEN** the system triggers the agent with the new commits as context

#### Scenario: No new commits
- **WHEN** the watched branch has no new commits
- **THEN** the system takes no action and continues polling

---

### Requirement: Context Assembly
The system SHALL assemble context for agents including: upstream diffs, upstream commit messages with concern tags, and the concern's prompt. This context MUST enable agents to respect upstream work.

#### Scenario: Context includes diffs and messages
- **WHEN** an agent is triggered for commits abc123 and def456
- **THEN** the context includes both commit messages and their diffs
- **AND** includes the concern's prompt
- **AND** includes instructions to respect upstream changes

---

### Requirement: Commit Tagging
All commits made by agents MUST include the concern name as a tag in the format `[{CONCERN_NAME}]` at the start of the commit message. The commit MUST also include a `Triggered-By:` trailer referencing the upstream commit.

#### Scenario: Agent makes changes
- **WHEN** the "security" concern agent commits a fix
- **THEN** the commit message starts with `[SECURITY]`
- **AND** includes `Triggered-By: {upstream_hash}` trailer

---

### Requirement: No-Change Handling
When an agent reviews code but makes no changes, the system SHALL fast-forward the output branch to match upstream and record the review via git notes in the format `[{CONCERN_NAME}] Reviewed, no changes needed`.

#### Scenario: Agent makes no changes
- **WHEN** the "security" concern agent finds no issues
- **THEN** the output branch is fast-forwarded to upstream
- **AND** a git note is added to the processed commits
- **AND** downstream concerns see the commits (via the advanced output branch)

---

### Requirement: Worktree Isolation
Each concern SHALL operate in its own git worktree. This MUST allow parallel execution of independent concerns without interference.

#### Scenario: Parallel execution
- **WHEN** concerns "linting" and "security" both watch "main"
- **THEN** they execute in separate worktrees
- **AND** changes in one worktree do not affect the other

---

### Requirement: Error Recovery
If an agent fails, the system SHALL log the error, skip updating the last-seen commit, and retry on the next poll. Other concerns MUST continue processing.

#### Scenario: Agent failure
- **WHEN** the "security" agent crashes
- **THEN** the error is logged
- **AND** the "security" concern retries on next poll
- **AND** other concerns continue normally

---

### Requirement: Status Display
The system SHALL provide a `detergent status` command that displays the concern graph with current state. Status indicators MUST include: caught up (✓), processing (⟳), waiting (◯), and failed (✗).

#### Scenario: Status command output
- **WHEN** user runs `detergent status`
- **THEN** the terminal displays the concern graph as ASCII art
- **AND** each concern shows its current status indicator
- **AND** each concern shows its last-processed commit hash

---

### Requirement: First Run Initialization
When a concern runs for the first time and its output branch doesn't exist, the system SHALL create the branch from the watched branch's current state and create the worktree.

#### Scenario: First run creates branch
- **WHEN** concern "security" runs for the first time
- **AND** branch `detergent/security` does not exist
- **THEN** the branch is created from the watched branch
- **AND** a worktree is created for the branch

