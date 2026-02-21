# Change: Add Assembly Line Daemon

## Why

Coding agents are powerful but unstructured. Multiple quality stations (security, deduplication, documentation) compete for attention in a single prompt, leading to inconsistent results. A pipeline approach lets each station focus deeply, with Git providing the audit trail.

## What Changes

- **NEW** `line` capability: A daemon that orchestrates coding agents through a station-based DAG
- Stations watch upstream branches and push to their own output branches
- Agents receive upstream context (diffs + commit messages) to preserve intent
- Git notes provide audit trail for no-change reviews
- Worktrees enable parallel, isolated station execution

## Impact

- Affected specs: `line` (new capability)
- Affected code: New daemon implementation (Go or similar)
- Dependencies: Git, Claude Code CLI (or configurable agent)
