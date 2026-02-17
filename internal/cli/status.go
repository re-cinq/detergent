package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/re-cinq/detergent/internal/config"
	"github.com/re-cinq/detergent/internal/engine"
	"github.com/re-cinq/detergent/internal/fileutil"
	gitops "github.com/re-cinq/detergent/internal/git"
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
	Use:   "status",
	Short: "Show the status of each concern",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, repoDir, err := loadConfigAndRepo(configPath)
		if err != nil {
			return err
		}

		if statusFollow {
			return followStatus(cfg, repoDir)
		}
		return showStatus(cfg, repoDir)
	},
}

func followStatus(cfg *config.Config, repoDir string) error {
	sigCh := setupSignalHandler()
	defer signal.Stop(sigCh)

	interval := time.Duration(statusInterval * float64(time.Second))
	first := true

	for {
		var buf bytes.Buffer
		if err := renderStatus(&buf, cfg, repoDir, true); err != nil {
			fileutil.LogError("\nerror: %s", err)
		}
		output := buf.String()

		// Move cursor home and clear screen on first render;
		// subsequent renders move home and overwrite in place
		// to avoid flicker while still refreshing log tails.
		if first {
			fmt.Print("\033[H\033[2J")
			first = false
		} else {
			fmt.Print("\033[H")
		}
		// Append clear-to-end-of-line escape to each line so shorter
		// lines fully overwrite longer ones from previous renders.
		lines := strings.Split(output, "\n")
		for i := range lines {
			lines[i] += "\033[K"
		}
		output = strings.Join(lines, "\n")

		fmt.Printf("Every %.1fs: detergent status\033[K\n\033[K\n", statusInterval)
		fmt.Print(output)
		// Clear from cursor to end of screen to remove stale lines
		fmt.Print("\033[J")

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

	fmt.Fprintln(w, "Concern Status")
	fmt.Fprintln(w, "──────────────────────────────────────")

	var activeConcerns []string

	for _, c := range cfg.Concerns {
		watchedBranch := engine.ResolveWatchedBranch(cfg, c)

		status, _ := engine.ReadStatus(repoDir, c.Name)

		// If actively processing, show the granular state
		if status != nil {
			// Check for stale active states (process died)
			if engine.IsActiveState(status.State) && !engine.IsProcessAlive(status.PID) {
				msg := fmt.Sprintf("stale (process %d no longer running, was: %s)", status.PID, status.State)
				fmt.Fprintln(w, formatStatus(engine.StateFailed, "", c.Name, msg))
				continue
			}

			switch status.State {
			case engine.StateChangeDetected:
				msg := fmt.Sprintf("change detected at %s", short(status.HeadAtStart))
				fmt.Fprintln(w, formatStatus(status.State, "", c.Name, msg))
				activeConcerns = append(activeConcerns, c.Name)
				continue
			case engine.StateAgentRunning:
				msg := fmt.Sprintf("agent running (since %s)", status.StartedAt)
				fmt.Fprintln(w, formatStatus(status.State, "", c.Name, msg))
				activeConcerns = append(activeConcerns, c.Name)
				continue
			case engine.StateCommitting:
				fmt.Fprintln(w, formatStatus(status.State, "", c.Name, "committing changes"))
				activeConcerns = append(activeConcerns, c.Name)
				continue
			case engine.StateFailed:
				msg := fmt.Sprintf("failed: %s", status.Error)
				fmt.Fprintln(w, formatStatus(status.State, "", c.Name, msg))
				continue
			case engine.StateSkipped:
				msg := fmt.Sprintf("skipped: %s", status.Error)
				fmt.Fprintln(w, formatStatus(status.State, "", c.Name, msg))
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
			msg := fmt.Sprintf("(not started - watched branch %s not found)", watchedBranch)
			fmt.Fprintln(w, formatStatus("unknown", "", c.Name, msg))
			continue
		}

		if lastSeen == "" {
			fmt.Fprintln(w, formatStatus("pending", "", c.Name, "pending (never processed)"))
		} else if lastSeen == head {
			msg := fmt.Sprintf("caught up at %s", short(lastSeen))
			fmt.Fprintln(w, formatStatus(engine.StateIdle, "result", c.Name, msg))
		} else {
			msg := fmt.Sprintf("pending (last: %s, head: %s)", short(lastSeen), short(head))
			fmt.Fprintln(w, formatStatus("pending", "", c.Name, msg))
		}
	}

	// In follow mode, show last few log lines for active concerns
	if showLogs && len(activeConcerns) > 0 {
		for _, name := range activeConcerns {
			logPath := engine.LogPathFor(name)
			tail := readLastLines(logPath, 5)
			if tail != "" {
				fmt.Fprintf(w, "\n%s── %s logs %s(Claude Code batches output) ──%s\n%s", ansiBoldMagenta, name, ansiDim, ansiReset, tail)
			}
		}
	}

	return nil
}

// readLastLines reads the last n lines from the most recent run in a log file.
// It finds the last "--- Processing" header and only considers lines after it,
// so that status -f doesn't show stale output from previous runs.
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

	// Find the last run header to skip previous runs' output
	runStart := 0
	for i, line := range lines {
		if strings.HasPrefix(line, "--- Processing ") {
			runStart = i + 1
		}
	}

	// Only show lines from the current run (after the header)
	if runStart >= len(lines) {
		return ""
	}
	lines = lines[runStart:]

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
