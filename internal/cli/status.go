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
		cfg, err := loadAndValidateConfig(configPath)
		if err != nil {
			return err
		}

		repoDir, err := resolveRepo(configPath)
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
				sym, clr := stateDisplay(engine.StateFailed, "")
				fmt.Fprintf(w, "  %s%s  %-20s  stale (process %d no longer running, was: %s)%s\n", clr, sym, c.Name, status.PID, status.State, ansiReset)
				continue
			}

			switch status.State {
			case engine.StateChangeDetected:
				sym, clr := stateDisplay(status.State, "")
				fmt.Fprintf(w, "  %s%s  %-20s  change detected at %s%s\n", clr, sym, c.Name, short(status.HeadAtStart), ansiReset)
				activeConcerns = append(activeConcerns, c.Name)
				continue
			case engine.StateAgentRunning:
				sym, clr := stateDisplay(status.State, "")
				fmt.Fprintf(w, "  %s%s  %-20s  agent running (since %s)%s\n", clr, sym, c.Name, status.StartedAt, ansiReset)
				activeConcerns = append(activeConcerns, c.Name)
				continue
			case engine.StateCommitting:
				sym, clr := stateDisplay(status.State, "")
				fmt.Fprintf(w, "  %s%s  %-20s  committing changes%s\n", clr, sym, c.Name, ansiReset)
				activeConcerns = append(activeConcerns, c.Name)
				continue
			case engine.StateFailed:
				sym, clr := stateDisplay(status.State, "")
				fmt.Fprintf(w, "  %s%s  %-20s  failed: %s%s\n", clr, sym, c.Name, status.Error, ansiReset)
				continue
			case engine.StateSkipped:
				sym, clr := stateDisplay(status.State, "")
				fmt.Fprintf(w, "  %s%s  %-20s  skipped: %s%s\n", clr, sym, c.Name, status.Error, ansiReset)
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
			sym, clr := stateDisplay("unknown", "")
			fmt.Fprintf(w, "  %s%s  %-20s  (not started - watched branch %s not found)%s\n", clr, sym, c.Name, watchedBranch, ansiReset)
			continue
		}

		if lastSeen == "" {
			sym, clr := stateDisplay("pending", "")
			fmt.Fprintf(w, "  %s%s  %-20s  pending (never processed)%s\n", clr, sym, c.Name, ansiReset)
		} else if lastSeen == head {
			sym, clr := stateDisplay(engine.StateIdle, "result")
			fmt.Fprintf(w, "  %s%s  %-20s  caught up at %s%s\n", clr, sym, c.Name, short(lastSeen), ansiReset)
		} else {
			sym, clr := stateDisplay("pending", "")
			fmt.Fprintf(w, "  %s%s  %-20s  pending (last: %s, head: %s)%s\n", clr, sym, c.Name, short(lastSeen), short(head), ansiReset)
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
