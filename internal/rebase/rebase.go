package rebase

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/re-cinq/assembly-line/internal/config"
	"github.com/re-cinq/assembly-line/internal/git"
)

// Options controls rebase behavior.
type Options struct {
	LeaveConflicts bool
}

// Result describes the outcome of a rebase operation.
type Result struct {
	Rebased       bool
	ChangedFiles  []string
	NothingToDo   bool
	Conflict      bool
	ConflictFiles []string
	Stashed       bool
	StashConflict bool
	Error         error
}

// Run performs a deterministic stash → rebase → unstash onto the terminal
// station branch. By default conflicts are aborted; with LeaveConflicts the
// rebase is left in progress with conflict markers in the working directory.
func Run(dir string, cfg *config.Config, opts Options) Result {
	if len(cfg.Stations) == 0 {
		return Result{NothingToDo: true}
	}

	// Guard: refuse to run if a rebase is already in progress.
	if rebaseInProgress(dir) {
		return Result{Error: fmt.Errorf("rebase already in progress")}
	}

	// Verify we're on the watched branch — rebasing any other branch
	// onto the terminal station would be wrong.
	branch, err := git.CurrentBranch(dir)
	if err != nil {
		return Result{Error: err}
	}
	if branch != cfg.Settings.Watches {
		return Result{Error: fmt.Errorf("must be on %s to rebase (currently on %s)", cfg.Settings.Watches, branch)}
	}

	// Terminal station is the last one in the pipeline.
	terminal := cfg.Stations[len(cfg.Stations)-1]
	terminalBranch := git.StationBranchName(terminal.Name)

	// Fast-path: branch doesn't exist.
	if !git.BranchExists(dir, terminalBranch) {
		return Result{NothingToDo: true}
	}

	// Fast-path: no commits to pick up.
	has, err := git.HasCommitsBetween(dir, cfg.Settings.Watches, terminalBranch)
	if err != nil {
		return Result{Error: err}
	}
	if !has {
		return Result{NothingToDo: true}
	}

	// Record pre-rebase HEAD so we can diff afterwards.
	oldHead, err := git.Run(dir, "rev-parse", "HEAD")
	if err != nil {
		return Result{Error: err}
	}

	// Stash if dirty.
	dirty, err := git.IsDirty(dir)
	if err != nil {
		return Result{Error: err}
	}
	if dirty {
		if err := git.StashPush(dir, "line-rebase: stashing WIP"); err != nil {
			return Result{Error: err}
		}
	}

	// Rebase onto terminal station.
	if err := git.Rebase(dir, terminalBranch); err != nil {
		if opts.LeaveConflicts {
			// Leave git in mid-rebase state with conflict markers.
			files, _ := git.ConflictedFiles(dir)
			return Result{
				Conflict:      true,
				ConflictFiles: files,
				Stashed:       dirty,
			}
		}
		// Default: abort rebase, restore stash, report.
		_ = git.RebaseAbort(dir)
		if dirty {
			_ = git.StashPop(dir)
		}
		return Result{Conflict: true}
	}

	// Restore stash if we created one.
	if dirty {
		if err := git.StashPop(dir); err != nil {
			return Result{StashConflict: true}
		}
	}

	// Determine what changed.
	files, err := git.DiffFiles(dir, oldHead, "HEAD")
	if err != nil {
		return Result{Error: err}
	}

	return Result{
		Rebased:      true,
		ChangedFiles: files,
	}
}

// rebaseInProgress returns true if .git/rebase-merge or .git/rebase-apply exists.
func rebaseInProgress(dir string) bool {
	for _, name := range []string{"rebase-merge", "rebase-apply"} {
		if info, err := os.Stat(filepath.Join(dir, ".git", name)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}
