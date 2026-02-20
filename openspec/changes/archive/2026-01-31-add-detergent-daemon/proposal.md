# Change: Add Assembly Line Daemon

## Why

Coding agents are powerful but unstructured. Multiple quality concerns (security, deduplication, documentation) compete for attention in a single prompt, leading to inconsistent results. A pipeline approach lets each concern focus deeply, with Git providing the audit trail.

## What Changes

- **NEW** `line` capability: A daemon that orchestrates coding agents through a concern-based DAG
- Concerns watch upstream branches and push to their own output branches
- Agents receive upstream context (diffs + commit messages) to preserve intent
- Git notes provide audit trail for no-change reviews
- Worktrees enable parallel, isolated concern execution

## Impact

- Affected specs: `line` (new capability)
- Affected code: New daemon implementation (Go or similar)
- Dependencies: Git, Claude Code CLI (or configurable agent)
