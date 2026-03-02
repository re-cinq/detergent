package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/re-cinq/assembly-line/internal/config"
	"github.com/re-cinq/assembly-line/internal/runner"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Stop all agents and reset station state",
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
		if !force {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return fmt.Errorf("refusing to clear without confirmation (use --force to skip)")
			}
			fmt.Print("This will stop all agents and reset all station state. Continue? [y/N] ")
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
				return fmt.Errorf("aborted")
			}
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		return runner.Clear(".", cfg)
	},
}

func init() {
	clearCmd.Flags().Bool("force", false, "skip confirmation prompt")
	rootCmd.AddCommand(clearCmd)
}
