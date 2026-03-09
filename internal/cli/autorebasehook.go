package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/re-cinq/assembly-line/internal/config"
	"github.com/re-cinq/assembly-line/internal/git"
	"github.com/re-cinq/assembly-line/internal/rebase"
	"github.com/re-cinq/assembly-line/internal/state"
	"github.com/spf13/cobra"
)

var autoRebaseHookCmd = &cobra.Command{
	Use:    "auto-rebase-hook",
	Short:  "PostToolUse hook for automatic rebase",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return nil // no config or invalid config — exit silently
		}

		if !cfg.Settings.AutoRebase {
			return nil
		}

		if len(cfg.Stations) == 0 {
			return nil
		}

		// Don't rebase while a line run is in progress — the git rebase
		// would trigger the post-commit hook, starting a new run that
		// kills the in-progress agents.
		if pid, _ := state.ReadPID("."); pid > 0 && state.IsProcessRunning(pid) {
			return nil
		}

		terminal := cfg.Stations[len(cfg.Stations)-1]
		terminalBranch := git.StationBranchName(terminal.Name)

		if !git.BranchExists(".", terminalBranch) {
			return nil
		}

		// Get terminal branch HEAD ref for dedup.
		terminalRef, err := git.Run(".", "rev-parse", terminalBranch)
		if err != nil {
			return nil
		}

		// Dedup: if we already prompted for this ref, exit silently.
		if state.ReadRebasePrompted(".") == terminalRef {
			return nil
		}

		// Check if there are actually commits to pick up.
		has, err := git.HasCommitsBetween(".", cfg.Settings.Watches, terminalBranch)
		if err != nil {
			return nil
		}
		if !has {
			return nil
		}

		r := rebase.Run(".", cfg, rebase.Options{
			LeaveConflicts: cfg.Settings.AutoResolve,
		})

		// Write the marker regardless of outcome to avoid re-attempting.
		_ = state.WriteRebasePrompted(".", terminalRef)

		if r.Rebased {
			msg := fmt.Sprintf("Auto-rebased onto %s.", terminalBranch)
			if len(r.ChangedFiles) > 0 {
				msg += fmt.Sprintf(" Changed files: %s", strings.Join(r.ChangedFiles, ", "))
			}
			return printBlockJSON(msg)
		}

		if r.Conflict {
			if cfg.Settings.AutoResolve {
				return printConflictBlockJSON(r, terminalBranch)
			}
			return printBlockJSON(fmt.Sprintf(
				"Auto-rebase onto %s had conflicts. Run `line rebase` manually to resolve.",
				terminalBranch,
			))
		}

		// StashConflict, NothingToDo, or error — exit silently.
		return nil
	},
}

func printConflictBlockJSON(r rebase.Result, terminalBranch string) error {
	msg := fmt.Sprintf("Auto-rebase onto %s has conflicts.", terminalBranch)
	if len(r.ConflictFiles) > 0 {
		msg += fmt.Sprintf(" Conflicted files: %s.", strings.Join(r.ConflictFiles, ", "))
	}
	msg += " To resolve: edit the conflicted files, then git add <resolved-files>, then git rebase --continue."
	if r.Stashed {
		msg += " After resolving, run git stash pop to restore your WIP."
	}
	return printBlockJSON(msg)
}

func printBlockJSON(reason string) error {
	out, err := json.Marshal(map[string]string{
		"decision": "block",
		"reason":   reason,
	})
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func init() {
	rootCmd.AddCommand(autoRebaseHookCmd)
}
