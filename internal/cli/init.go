package cli

import (
	"fmt"

	"github.com/re-cinq/assembly-line/internal/gitignore"
	"github.com/re-cinq/assembly-line/internal/hooks"
	"github.com/re-cinq/assembly-line/internal/settings"
	"github.com/re-cinq/assembly-line/internal/skill"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Install assembly-line git hooks and skills in the current repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := hooks.Install("."); err != nil {
			return fmt.Errorf("installing hooks: %w", err)
		}
		if err := skill.Install("."); err != nil {
			return fmt.Errorf("installing skills: %w", err)
		}
		if err := settings.ConfigureStatusline("."); err != nil {
			return fmt.Errorf("configuring statusline: %w", err)
		}
		if err := settings.ConfigureAutoRebaseHook("."); err != nil {
			return fmt.Errorf("configuring auto-rebase hook: %w", err)
		}
		if err := gitignore.Install("."); err != nil {
			return fmt.Errorf("configuring gitignore: %w", err)
		}
		fmt.Println("assembly-line initialized")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
