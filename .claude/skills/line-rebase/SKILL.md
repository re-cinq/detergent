# /line-rebase

Safely pick up changes from the terminal station branch onto the watched branch.

## When to use

Use this skill when the line statusline indicates there are commits ready on the terminal station branch that haven't been picked up on the main working branch.

## Procedure

Run the deterministic `line rebase` command:

```sh
line rebase
```

This performs a safe stash → rebase → unstash sequence. It will:
- Stash any uncommitted work
- Rebase onto the terminal station branch
- Restore stashed work
- Report changed files on success, or abort cleanly on conflict

## Safety guarantees

- **No work is ever lost**: WIP is always stashed before any branch operations
- **No retriggering**: Station commits contain `[skip line]` which prevents `line run` from retriggering (RUN-9, SKL-2)
- **No auto-resolution**: If the rebase has conflicts, it aborts and restores your working state
