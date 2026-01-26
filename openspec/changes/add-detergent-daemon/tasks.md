# Implementation Tasks

Each slice delivers end-to-end user value with an acceptance test driving the binary.

## Slice 1: I can run the CLI
- [x] 1.1 Set up Go module, basic CLI structure (cobra or similar)
- [x] 1.2 `detergent --help` shows available commands
- [x] 1.3 `detergent --version` shows version
- [x] 1.4 **Acceptance test:** invoke binary, verify help output and exit code 0

## Slice 2: I can validate my config
- [x] 2.1 Define YAML config schema (concerns, watches, prompts)
- [x] 2.2 `detergent validate <config>` parses and validates config
- [x] 2.3 Reports clear errors for invalid YAML, missing fields, unknown references
- [x] 2.4 Detects cycles in concern graph
- [x] 2.5 **Acceptance test:** valid config exits 0, invalid configs exit non-zero with helpful message

## Slice 3: I can see my concern graph
- [x] 3.1 `detergent viz <config>` outputs ASCII DAG of concerns
- [x] 3.2 Shows concern names and what each watches
- [x] 3.3 **Acceptance test:** invoke viz, verify output matches expected graph structure

## Slice 4: I can run one pass manually
- [x] 4.1 `detergent run --once <config>` processes pending commits once, then exits
- [x] 4.2 Creates worktree for concern if needed
- [x] 4.3 Creates output branch if needed (from watched branch)
- [x] 4.4 Assembles context (diffs, commit messages, prompt)
- [x] 4.5 Invokes agent CLI with context
- [x] 4.6 Commits agent changes with `[CONCERN]` tag and `Triggered-By:` trailer
- [x] 4.7 **Acceptance test:** set up git repo, push commit, run once, verify agent was invoked and commit appears on output branch

## Slice 5: I can see what happened
- [x] 5.1 `detergent status <config>` shows concern states
- [x] 5.2 Shows last-processed commit per concern
- [x] 5.3 Shows status indicators (✓ caught up, ◯ pending, ✗ failed)
- [x] 5.4 **Acceptance test:** run once, then status, verify output reflects processed state

## Slice 6: I can run continuously
- [x] 6.1 `detergent run <config>` polls at configurable interval
- [x] 6.2 Detects new commits and processes them
- [x] 6.3 Runs until interrupted (SIGINT/SIGTERM)
- [x] 6.4 **Acceptance test:** start daemon, push commit, verify processing, send SIGINT, verify clean exit

## Slice 7: Concerns chain together
- [x] 7.1 Concern B watches concern A's output branch
- [x] 7.2 When A commits, B is triggered on next poll
- [x] 7.3 Context for B includes A's commit message (intent preservation)
- [x] 7.4 **Acceptance test:** config with A→B chain, push to source, verify both process in order

## Slice 8: No-change reviews leave a trace
- [ ] 8.1 When agent makes no changes, fast-forward output branch
- [ ] 8.2 Add git note to processed commits: `[CONCERN] Reviewed, no changes needed`
- [ ] 8.3 Downstream concerns still see the commits (branch advanced)
- [ ] 8.4 **Acceptance test:** agent returns no changes, verify fast-forward and git note exists

## Slice 9: One failure doesn't stop everything
- [ ] 9.1 If agent fails, log error and mark concern as failed
- [ ] 9.2 Don't advance last-seen commit (retry on next poll)
- [ ] 9.3 Other concerns continue processing independently
- [ ] 9.4 **Acceptance test:** config with A and B (independent), A's agent fails, verify B still processes

## Slice 10: Parallel concerns run in parallel
- [ ] 10.1 Independent concerns (no dependency) execute concurrently
- [ ] 10.2 Dependent concerns wait for upstream to complete
- [ ] 10.3 **Acceptance test:** config with parallel branches, verify concurrent execution (timing or log interleaving)
