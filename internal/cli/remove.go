package cli

import (
	"fmt"

	"github.com/re-cinq/assembly-line/internal/gitignore"
	"github.com/re-cinq/assembly-line/internal/hooks"
	"github.com/re-cinq/assembly-line/internal/settings"
	"github.com/re-cinq/assembly-line/internal/skill"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove assembly-line git hooks, skills, and configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := hooks.Remove("."); err != nil {
			return fmt.Errorf("removing hooks: %w", err)
		}
		if err := skill.Remove("."); err != nil {
			return fmt.Errorf("removing skills: %w", err)
		}
		if err := settings.RemoveStatusline("."); err != nil {
			return fmt.Errorf("removing statusline config: %w", err)
		}
		if err := settings.RemoveAutoRebaseHook("."); err != nil {
			return fmt.Errorf("removing auto-rebase hook: %w", err)
		}
		if err := gitignore.Remove("."); err != nil {
			return fmt.Errorf("removing gitignore entries: %w", err)
		}
		fmt.Println("assembly-line removed")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(removeCmd)
}
