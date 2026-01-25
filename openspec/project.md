# Project Context

## Purpose
Detergent is a daemon that orchestrates coding agents in a concern-based pipeline. Each agent focuses on a single quality concern (security, deduplication, style, etc.). Changes flow through a directed graph of concerns, with Git providing the audit trail and intent preservation.

## Tech Stack
- Go (daemon implementation)
- Git (version control, worktrees, notes)
- YAML (configuration)
- Claude Code CLI (default agent, configurable)
- Ginkgo + Gomega (testing framework)

## Project Conventions

### Code Style
- Go standard formatting (`gofmt`)
- Error handling: wrap errors with context
- Logging: structured logging with levels

### Architecture Patterns
- DAG for concern ordering
- Polling model for branch watching
- Worktree isolation per concern
- Git notes for audit trail

### Testing Strategy
- **Framework:** Ginkgo + Gomega, BDD/RSpec style
- **Structure:** `Describe`/`Context`/`It` blocks with descriptive specs
- **Style:** Favor readability; specs should read like documentation

### Development Approach
- **Thin vertical slices:** Each increment delivers end-to-end functionality
- **Outside-in:** Acceptance tests drive the compiled binary; no mocks at the boundary
- **No horizontal layers:** Don't build "all config parsing" then "all git ops" - build working features
- **Each slice = acceptance test:** Work isn't done until there's a passing test exercising the binary

### Git Workflow
- Trunk-based development on `main`
- Feature branches for significant changes
- Commit messages: imperative mood, concise

## Domain Context
- **Concern**: A single-purpose agent focus (e.g., "fix security issues")
- **Concern Graph**: DAG of concerns, upstream to downstream
- **Intent Preservation**: Downstream agents respect upstream work via commit context
- **Implicit Priority**: Position in graph implies precedence

## Important Constraints
- Must work with standard Git (no special server)
- Must handle agent failures gracefully
- Must not lose commits or create orphan branches
- Must provide clear audit trail via git log

## External Dependencies
- Git CLI (2.20+ for worktree features)
- Claude Code CLI (or configurable alternative)
