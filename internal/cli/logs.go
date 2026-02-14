package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/fission-ai/detergent/internal/config"
	"github.com/fission-ai/detergent/internal/engine"
	"github.com/spf13/cobra"
)

var (
	logsFollow bool
	logsTail   int
)

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output (like tail -f)")
	logsCmd.Flags().IntVarP(&logsTail, "tail", "n", 50, "Number of lines to show")
	rootCmd.AddCommand(logsCmd)
}

var logsCmd = &cobra.Command{
	Use:   "logs <config-file> <concern>",
	Short: "Show agent logs for a concern",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(args[0])
		if err != nil {
			return err
		}

		concernName := args[1]

		// Validate concern name exists in config
		found := false
		for _, c := range cfg.Concerns {
			if c.Name == concernName {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown concern %q", concernName)
		}

		logPath := engine.LogPathFor(concernName)
		if _, err := os.Stat(logPath); os.IsNotExist(err) {
			return fmt.Errorf("no log file found for %q (expected at %s)", concernName, logPath)
		}

		// Use tail to display the log
		tailArgs := []string{"-n", fmt.Sprintf("%d", logsTail)}
		if logsFollow {
			tailArgs = append(tailArgs, "-f")
		}
		tailArgs = append(tailArgs, logPath)

		tailCmd := exec.Command("tail", tailArgs...)
		tailCmd.Stdout = os.Stdout
		tailCmd.Stderr = os.Stderr
		return tailCmd.Run()
	},
}
