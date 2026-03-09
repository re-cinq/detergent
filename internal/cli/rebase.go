package cli

import (
	"fmt"
	"strings"

	"github.com/re-cinq/assembly-line/internal/config"
	"github.com/re-cinq/assembly-line/internal/git"
	"github.com/re-cinq/assembly-line/internal/rebase"
	"github.com/spf13/cobra"
)

var leaveConflicts bool

var rebaseCmd = &cobra.Command{
	Use:   "rebase",
	Short: "Rebase onto the terminal station branch to pick up line changes",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}

		r := rebase.Run(".", cfg, rebase.Options{
			LeaveConflicts: leaveConflicts,
		})

		if r.Error != nil {
			return r.Error
		}
		if r.NothingToDo {
			fmt.Println("Nothing to rebase — already up to date.")
			return nil
		}
		if r.Conflict {
			if leaveConflicts {
				return printConflictInstructions(r)
			}
			fmt.Println("Rebase aborted due to conflicts. Working state preserved.")
			return fmt.Errorf("rebase conflict")
		}
		if r.StashConflict {
			fmt.Println("Stash pop failed after rebase — your WIP may need manual resolution.")
			return fmt.Errorf("stash pop conflict")
		}

		terminal := cfg.Stations[len(cfg.Stations)-1]
		terminalBranch := git.StationBranchName(terminal.Name)
		fmt.Printf("Rebased onto %s.", terminalBranch)
		if len(r.ChangedFiles) > 0 {
			fmt.Printf(" Changed files: %s", strings.Join(r.ChangedFiles, ", "))
		}
		fmt.Println()

		return nil
	},
}

func printConflictInstructions(r rebase.Result) error {
	fmt.Println("Rebase paused — conflicts need resolution.")
	if len(r.ConflictFiles) > 0 {
		fmt.Printf("Conflicted files: %s\n", strings.Join(r.ConflictFiles, ", "))
	}
	fmt.Println()
	fmt.Println("To resolve:")
	fmt.Println("  1. Edit the conflicted files to resolve the conflict markers")
	fmt.Println("  2. git add <resolved-files>")
	fmt.Println("  3. git rebase --continue")
	if r.Stashed {
		fmt.Println("  4. git stash pop")
	}
	return fmt.Errorf("rebase conflict (left for resolution)")
}

func init() {
	rebaseCmd.Flags().BoolVar(&leaveConflicts, "leave-conflicts", false,
		"Leave git in mid-rebase state with conflict markers instead of aborting")
	rootCmd.AddCommand(rebaseCmd)
}
