package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fission-ai/detergent/internal/config"
	"github.com/fission-ai/detergent/internal/engine"
	gitops "github.com/fission-ai/detergent/internal/git"
	"github.com/spf13/cobra"
)

var (
	statusFollow   bool
	statusInterval float64
)

func init() {
	statusCmd.Flags().BoolVarP(&statusFollow, "follow", "f", false, "Live-update status (like watch)")
	statusCmd.Flags().Float64VarP(&statusInterval, "interval", "n", 2.0, "Seconds between updates (with --follow)")
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status <config-file>",
	Short: "Show the status of each concern",
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

		configPath, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		repoDir := findGitRoot(filepath.Dir(configPath))
		if repoDir == "" {
			return fmt.Errorf("could not find git repository root")
		}

		if statusFollow {
			return followStatus(cfg, repoDir)
		}
		return showStatus(cfg, repoDir)
	},
}

func followStatus(cfg *config.Config, repoDir string) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	interval := time.Duration(statusInterval * float64(time.Second))
	var lastOutput string

	for {
		var buf bytes.Buffer
		if err := renderStatus(&buf, cfg, repoDir, true); err != nil {
			fmt.Fprintf(os.Stderr, "\nerror: %s\n", err)
		}
		output := buf.String()

		if output != lastOutput {
			fmt.Print("\033[H\033[2J")
			fmt.Printf("Every %.1fs: detergent status\n\n", statusInterval)
			fmt.Print(output)
			lastOutput = output
		}

		select {
		case <-sigCh:
			fmt.Println()
			return nil
		case <-time.After(interval):
		}
	}
}

func showStatus(cfg *config.Config, repoDir string) error {
	return renderStatus(os.Stdout, cfg, repoDir, false)
}

func renderStatus(w io.Writer, cfg *config.Config, repoDir string, showLogs bool) error {
	repo := gitops.NewRepo(repoDir)
	nameSet := make(map[string]bool)
	for _, c := range cfg.Concerns {
		nameSet[c.Name] = true
	}

	fmt.Fprintln(w, "Concern Status")
	fmt.Fprintln(w, "──────────────────────────────────────")

	var activeConcerns []string

	for _, c := range cfg.Concerns {
		watchedBranch := c.Watches
		if nameSet[c.Watches] {
			watchedBranch = cfg.Settings.BranchPrefix + c.Watches
		}

		status, _ := engine.ReadStatus(repoDir, c.Name)

		// If actively processing, show the granular state
		if status != nil {
			// Check for stale active states (process died)
			if engine.IsActiveState(status.State) && !engine.IsProcessAlive(status.PID) {
				fmt.Fprintf(w, "  ✗  %-20s  stale (process %d no longer running, was: %s)\n", c.Name, status.PID, status.State)
				continue
			}

			switch status.State {
			case "change_detected":
				fmt.Fprintf(w, "  ◎  %-20s  change detected at %s\n", c.Name, short(status.HeadAtStart))
				activeConcerns = append(activeConcerns, c.Name)
				continue
			case "agent_running":
				fmt.Fprintf(w, "  ⟳  %-20s  agent running (since %s)\n", c.Name, status.StartedAt)
				activeConcerns = append(activeConcerns, c.Name)
				continue
			case "committing":
				fmt.Fprintf(w, "  ⟳  %-20s  committing changes\n", c.Name)
				activeConcerns = append(activeConcerns, c.Name)
				continue
			case "failed":
				fmt.Fprintf(w, "  ✗  %-20s  failed: %s\n", c.Name, status.Error)
				continue
			case "skipped":
				fmt.Fprintf(w, "  ⊘  %-20s  skipped: %s\n", c.Name, status.Error)
				continue
			}
		}

		lastSeen, err := engine.LastSeen(repoDir, c.Name)
		if err != nil {
			return err
		}

		head, err := repo.HeadCommit(watchedBranch)
		if err != nil {
			// Branch might not exist yet
			fmt.Fprintf(w, "  ◯  %-20s  (not started - watched branch %s not found)\n", c.Name, watchedBranch)
			continue
		}

		if lastSeen == "" {
			fmt.Fprintf(w, "  ◯  %-20s  pending (never processed)\n", c.Name)
		} else if lastSeen == head {
			fmt.Fprintf(w, "  ✓  %-20s  caught up at %s\n", c.Name, short(lastSeen))
		} else {
			fmt.Fprintf(w, "  ◯  %-20s  pending (last: %s, head: %s)\n", c.Name, short(lastSeen), short(head))
		}
	}

	// In follow mode, show last few log lines for active concerns
	if showLogs && len(activeConcerns) > 0 {
		for _, name := range activeConcerns {
			logPath := engine.LogPathFor(name)
			tail := readLastLines(logPath, 5)
			if tail != "" {
				fmt.Fprintf(w, "\n── %s logs ──\n%s", name, tail)
			}
		}
	}

	return nil
}

// readLastLines reads the last n lines from a file, returning "" if the file doesn't exist.
func readLastLines(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	content := strings.TrimRight(string(data), "\n")
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n") + "\n"
}

func short(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}
