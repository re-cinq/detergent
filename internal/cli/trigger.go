package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/re-cinq/assembly-line/internal/engine"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(triggerCmd)
}

var triggerCmd = &cobra.Command{
	Use:    "trigger",
	Short:  "Write trigger file and start runner if needed",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Don't re-trigger when running inside a line agent worktree
		if os.Getenv("LINE_AGENT") != "" {
			return nil
		}

		_, repoDir, err := loadConfigAndRepo(configPath)
		if err != nil {
			return err
		}

		// Get HEAD commit hash
		gitCmd := exec.Command("git", "rev-parse", "HEAD")
		gitCmd.Dir = repoDir
		out, err := gitCmd.Output()
		if err != nil {
			return fmt.Errorf("getting HEAD: %w", err)
		}
		head := strings.TrimSpace(string(out))

		// Write the trigger file
		if err := engine.WriteTrigger(repoDir, head); err != nil {
			return fmt.Errorf("writing trigger: %w", err)
		}

		// If no runner is alive, spawn one detached
		if !engine.IsRunnerAlive(repoDir) {
			self, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolving self: %w", err)
			}

			runCmd := exec.Command(self, "run", "--path", configPath)
			runCmd.Dir = repoDir
			runCmd.Stdin = nil
			runCmd.Stdout = nil
			runCmd.Stderr = nil
			runCmd.SysProcAttr = detachedProcAttr()

			// Strip env vars that interfere with the runner:
			// - CLAUDECODE: so Claude agents don't refuse to start
			// - GIT_DIR/GIT_INDEX_FILE/GIT_WORK_TREE: set by git during
			//   hook execution, they override the worktree's own git
			//   context and cause "index file open failed: Not a directory"
			for _, e := range os.Environ() {
				if strings.HasPrefix(e, "CLAUDECODE=") ||
					strings.HasPrefix(e, "GIT_DIR=") ||
					strings.HasPrefix(e, "GIT_INDEX_FILE=") ||
					strings.HasPrefix(e, "GIT_WORK_TREE=") {
					continue
				}
				runCmd.Env = append(runCmd.Env, e)
			}

			if err := runCmd.Start(); err != nil {
				return fmt.Errorf("spawning runner: %w", err)
			}
			if err := runCmd.Process.Release(); err != nil {
				return fmt.Errorf("detaching runner: %w", err)
			}
		}

		return nil
	},
}
