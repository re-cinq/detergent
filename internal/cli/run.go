package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

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
		cfg, err := loadAndValidateConfig(args[0])
		if err != nil {
			return err
		}

		repoDir, err := resolveRepo(args[0])
		if err != nil {
			return err
		}

		if runOnce {
			return engine.RunOnce(cfg, repoDir)
		}

		return runDaemon(cfg, repoDir)
	},
}

func runDaemon(cfg *config.Config, repoDir string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Create LogManager for daemon lifetime
	logMgr := engine.NewLogManager()
	defer logMgr.Close()

	fmt.Printf("detergent daemon started (polling every %s)\n", cfg.Settings.PollInterval.Duration())
	fmt.Printf("Agent logs: %s\n", engine.LogPath())

	ticker := time.NewTicker(cfg.Settings.PollInterval.Duration())
	defer ticker.Stop()

	// Run immediately on startup
	if err := engine.RunOnceWithLogs(cfg, repoDir, logMgr); err != nil {
		fmt.Fprintf(os.Stderr, "poll error: %s\n", err)
	}

	for {
		select {
		case <-ctx.Done():
			fmt.Println("detergent daemon stopped")
			return nil
		case sig := <-sigCh:
			fmt.Printf("\nreceived %s, shutting down...\n", sig)
			cancel()
		case <-ticker.C:
			if err := engine.RunOnceWithLogs(cfg, repoDir, logMgr); err != nil {
				fmt.Fprintf(os.Stderr, "poll error: %s\n", err)
			}
		}
	}
}
