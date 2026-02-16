package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/re-cinq/detergent/internal/config"
	"github.com/re-cinq/detergent/internal/engine"
	"github.com/re-cinq/detergent/internal/fileutil"
	"github.com/spf13/cobra"
)

var runOnce bool

func init() {
	runCmd.Flags().BoolVar(&runOnce, "once", false, "Process pending commits once and exit")
	rootCmd.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the detergent daemon",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadAndValidateConfig(configPath)
		if err != nil {
			return err
		}

		repoDir, err := resolveRepo(configPath)
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

	sigCh := setupSignalHandler()

	// Create LogManager for daemon lifetime
	logMgr := engine.NewLogManager()
	defer logMgr.Close()

	fmt.Printf("detergent daemon started (polling every %s)\n", cfg.Settings.PollInterval.Duration())
	fmt.Printf("Agent logs: %s\n", engine.LogPath())

	ticker := time.NewTicker(cfg.Settings.PollInterval.Duration())
	defer ticker.Stop()

	// Run immediately on startup
	if err := engine.RunOnceWithLogs(cfg, repoDir, logMgr); err != nil {
		fileutil.LogError("poll error: %s", err)
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
			cfg = reloadConfig(configPath, cfg, ticker)
			if err := engine.RunOnceWithLogs(cfg, repoDir, logMgr); err != nil {
				fileutil.LogError("poll error: %s", err)
			}
		}
	}
}

// reloadConfig attempts to reload and validate the config file.
// If successful and the poll interval changed, the ticker is reset.
// On any error, the previous config is returned unchanged.
func reloadConfig(path string, prev *config.Config, ticker *time.Ticker) *config.Config {
	newCfg, err := config.Load(path)
	if err != nil {
		fileutil.LogError("config reload: %s (keeping previous config)", err)
		return prev
	}
	if errs := config.Validate(newCfg); len(errs) > 0 {
		fileutil.LogError("config reload: invalid (%s) (keeping previous config)", errs[0])
		return prev
	}

	if newCfg.Settings.PollInterval != prev.Settings.PollInterval {
		ticker.Reset(newCfg.Settings.PollInterval.Duration())
		fmt.Printf("config reloaded: poll interval changed to %s\n", newCfg.Settings.PollInterval.Duration())
	}

	return newCfg
}
