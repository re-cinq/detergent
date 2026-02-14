---
name: rebase
description: Rebase main with the output of a completed detergent concern chain. Handles backup, stash, conflict resolution, and recovery.
metadata:
  author: detergent
  version: "2.0"
---

Merge the results of a completed detergent concern chain back into the main branch.

After detergent processes commits through a chain of concerns (e.g., security → style → docs), the terminal concern's branch holds the accumulated agent output. This skill rebases main onto that branch so the agent's work lands cleanly.

---

## Phase 1: Discover the Concern Chain

1. **Find `detergent.yaml`**
   Look in the repo root, then walk up:
   ```bash
   git rev-parse --show-toplevel
   ```
   Then check for `detergent.yaml` or `detergent.yml` in that directory.
   Read the file.

2. **Parse the concern chain**
   From the YAML config, identify:
   - The **branch prefix** (from `settings.branch_prefix`, default: `detergent/`)
   - All concerns and their `watches` fields
   - The **terminal concern**: the concern whose name does not appear in any other concern's `watches` field

   For a linear chain like:
   ```yaml
   concerns:
     - name: security
       watches: main
     - name: style
       watches: security
     - name: docs
       watches: style
   ```
   The terminal concern is `docs` (nothing watches it).

   If there are **multiple terminal concerns** (branching graph), STOP and tell the user:
   "Multiple terminal concerns detected (<names>). This skill only supports linear chains. Please specify which branch to rebase onto."

3. **Derive the terminal branch name**
   ```
   <branch_prefix><terminal_concern_name>
   ```
   e.g., `detergent/docs`

4. **Verify the chain is complete**
   Check `.detergent/status/<concern>.json` for each concern in the chain.
   - If any concern has `"state": "running"`, STOP: "Concern chain is still running (<name> is active). Wait for it to finish."
   - If any concern has `"state": "failed"`, STOP: "Concern <name> failed. Check logs before rebasing."
   - Verify the terminal branch exists:
     ```bash
     git rev-parse --verify <terminal-branch> 2>/dev/null
     ```
     If it doesn't exist, STOP: "Terminal branch `<name>` does not exist. Has detergent run yet?"

---

## Phase 2: Preflight

5. **Verify we're on the main branch**
   ```bash
   git branch --show-current
   ```
   The current branch should be the branch that the first concern watches (typically `main` or `master`).
   If not, ask the user: "You're on `<branch>`. Switch to `<main-branch>` first?"

6. **Check for uncommitted changes**
   ```bash
   git status --porcelain
   ```
   Store whether there are changes as `HAD_CHANGES`.

7. **Check if there's anything to do**
   ```bash
   git log --oneline <terminal-branch>..HEAD
   git log --oneline HEAD..<terminal-branch>
   ```
   - If the terminal branch is identical to HEAD: STOP: "Main is already up to date with the concern chain."
   - If the terminal branch is behind HEAD (ancestor of HEAD): STOP: "The concern chain hasn't produced new changes since your last rebase."

---

## Phase 3: Safety Net

8. **Create backup branch**
   ```bash
   git branch -f pre-rebase-backup
   ```
   Report: "Created backup at `pre-rebase-backup` (SHORTSHA)"

9. **Stash if needed**
   If `HAD_CHANGES` is true:
   ```bash
   git stash push -m "detergent-rebase-autostash"
   ```
   Store `DID_STASH=true`. If the stash fails, STOP — do not proceed with dirty state.

---

## Phase 4: Rebase

10. **Rebase main onto the terminal branch**
    ```bash
    git rebase <terminal-branch>
    ```
    This replays any commits on main that aren't in the terminal branch on top of the agent's accumulated work.

    - If clean (exit 0): skip to Phase 6
    - If conflicts: proceed to Phase 5

---

## Phase 5: Conflict Resolution (loop)

Repeat until the rebase completes or is aborted. Track `CONFLICT_ROUND` starting at 0.

11. **List conflicted files**
    ```bash
    git diff --name-only --diff-filter=U
    ```

12. **For each conflicted file:**
    a. **Read** the full file (it contains conflict markers)
    b. **Understand both sides:**
       - `<<<<<<<` to `=======` is your commit being replayed (the developer's changes)
       - `=======` to `>>>>>>>` is the agent's accumulated output (from the concern chain)
    c. **Resolve intelligently:**
       - For changes the agent made intentionally (matching concern scope — security fixes, style changes, etc.): **prefer the agent's version**
       - For changes the developer made that don't conflict with the agent's intent: **preserve the developer's version**
       - When both sides modified the same lines for different reasons: **combine them**, keeping the agent's structural changes while preserving the developer's business logic
       - **Remove ALL conflict markers** — no `<<<<<<<`, `=======`, or `>>>>>>>` may remain
    d. **Verify** no remaining conflict markers:
       ```bash
       grep -c '<<<<<<<\|=======\|>>>>>>>' <file> || true
       ```
       If any remain, re-resolve.
    e. **Stage** the resolved file:
       ```bash
       git add <file>
       ```

13. **Continue rebase**
    ```bash
    GIT_EDITOR=true git rebase --continue
    ```
    - If more conflicts appear: increment `CONFLICT_ROUND`, return to step 11
    - If clean: proceed to Phase 6
    - If "nothing to commit" error: `git rebase --skip` and continue

**ABORT CONDITION**: If `CONFLICT_ROUND` exceeds 10, abort:
```bash
git rebase --abort
```
Tell the user: "Rebase aborted after too many conflict rounds. Your branch is restored. Backup: `pre-rebase-backup`."

---

## Phase 6: Restore Stash

14. **Pop stash if we stashed**
    If `DID_STASH` is true:
    ```bash
    git stash pop
    ```
    - If conflicts during pop: resolve them, then stage
    - If pop fails entirely: do NOT drop the stash. Tell the user their changes are safe in `git stash list`.

---

## Phase 7: Report

15. **Show summary**
    ```bash
    git log --oneline pre-rebase-backup..HEAD
    ```

    Report to the user:

    ```
    Rebase complete.

    - Concern chain: <concern1> → <concern2> → ... → <terminal>
    - Terminal branch: <terminal-branch>
    - Commits replayed: <N> (or "fast-forward" if none)
    - Conflicts resolved: <count> (or "none")
    - Stash: restored (or "nothing to restore")
    - Backup: `pre-rebase-backup` at <shortsha>
    - To undo: git reset --hard pre-rebase-backup
    ```

---

## Guardrails

- **NEVER** force-push. This skill only performs local operations.
- **NEVER** delete the `pre-rebase-backup` branch.
- **ALWAYS** create the backup before any destructive operation.
- **ALWAYS** set `GIT_EDITOR=true` when running `git rebase --continue`.
- **ALWAYS** verify the concern chain is complete before rebasing. Never rebase mid-chain.
- If the rebase is aborted, verify the branch is restored to its pre-rebase state.
- If anything unexpected happens, prefer aborting over continuing blindly.
- This skill only supports **linear chains** (single terminal concern). Branching graphs require manual intervention.
