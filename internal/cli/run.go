package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fission-ai/detergent/internal/config"
	"github.com/fission-ai/detergent/internal/engine"
	"github.com/spf13/cobra"
)

var runOnce bool

func init() {
	runCmd.Flags().BoolVar(&runOnce, "once", false, "Process pending commits once and exit")
	rootCmd.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run <config-file>",
	Short: "Run the detergent daemon",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			return err
		}

		errs := config.Validate(cfg)
		if len(errs) > 0 {
			for _, e := range errs {
				fmt.Fprintf(os.Stderr, "Error: %s\n", e)
			}
			return fmt.Errorf("%d validation error(s)", len(errs))
		}

		// Resolve the repo directory (directory containing the config file)
		configPath, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		repoDir := findGitRoot(filepath.Dir(configPath))
		if repoDir == "" {
			return fmt.Errorf("could not find git repository root from %s", filepath.Dir(configPath))
		}

		if runOnce {
			return engine.RunOnce(cfg, repoDir)
		}

		// Daemon mode (Slice 6)
		return fmt.Errorf("daemon mode not yet implemented, use --once")
	},
}

func findGitRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
