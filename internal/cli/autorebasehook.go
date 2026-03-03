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

		r := rebase.Run(".", cfg)

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
			return printBlockJSON(fmt.Sprintf(
				"Auto-rebase onto %s had conflicts. Run `line rebase` manually to resolve.",
				terminalBranch,
			))
		}

		// StashConflict, NothingToDo, or error — exit silently.
		return nil
	},
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
