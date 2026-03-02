package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/re-cinq/assembly-line/internal/config"
	"github.com/re-cinq/assembly-line/internal/git"
	"github.com/re-cinq/assembly-line/internal/runner"
	"github.com/re-cinq/assembly-line/internal/state"
	"github.com/re-cinq/assembly-line/internal/tmux"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// ANSI color codes for STAT-2 color coding
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorOrange = "\033[33m"
	colorYellow = "\033[93m"
	colorRed    = "\033[31m"
	colorGrey   = "\033[90m"
)

var followFlag bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of the assembly line",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}

		if followFlag {
			// Hide cursor and clear screen during follow mode; restore on exit or signal
			fmt.Print("\033[?25l\033[2J")
			showCursor := func() { fmt.Print("\033[?25h") }
			defer showCursor()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt)
			go func() {
				<-sigCh
				showCursor()
				os.Exit(0)
			}()
		}

		for {
			if followFlag {
				// Move cursor to home position (no screen clear to avoid flicker)
				fmt.Print("\033[H")
			}

			if err := printStatus(".", cfg, followFlag); err != nil {
				return err
			}

			if followFlag {
				// Clear from cursor to end of screen (remove stale lines)
				fmt.Print("\033[J\033[?25l")
			}

			if !followFlag {
				break
			}
			time.Sleep(2 * time.Second)
		}
		return nil
	},
}

// stationInfo holds the computed display state for a station.
type stationInfo struct {
	symbol    string
	color     string
	name      string    // "pending", "agent running", "failed", "up to date"
	startTime time.Time // non-zero when agent is running
}

// computeStationInfo returns the display state for a station based on process
// and git state (STAT-5: on-demand computation).
func computeStationInfo(dir string, station config.Station, watchedFullRef, watchedBranch string) stationInfo {
	branchName := git.StationBranchName(station.Name)
	if !git.BranchExists(dir, branchName) {
		return stationInfo{symbol: "○", color: colorYellow, name: "pending"}
	}

	agentPID, startTime, _ := state.ReadStationPID(dir, station.Name)
	if agentPID > 0 && state.IsProcessRunning(agentPID) {
		return stationInfo{symbol: "●", color: colorOrange, name: "agent running", startTime: startTime}
	}
	if state.ReadStationFailed(dir, station.Name) {
		return stationInfo{symbol: "✗", color: colorRed, name: "failed"}
	}
	if watchedFullRef != "" && git.IsAncestor(dir, watchedFullRef, branchName) {
		return stationInfo{symbol: "✓", color: colorGreen, name: "up to date"}
	}
	// STAT-8: If the only commits between station and watched branch are
	// skip-marker commits, the station is still up to date.
	if watchedFullRef != "" && git.OnlySkipCommitsBetween(dir, branchName, watchedBranch, runner.SkipMarkers) {
		return stationInfo{symbol: "✓", color: colorGreen, name: "up to date"}
	}
	return stationInfo{symbol: "○", color: colorYellow, name: "pending"}
}

// formatUptime formats the duration since startTime as a human-readable string.
func formatUptime(startTime time.Time) string {
	d := time.Since(startTime)
	s := int(d.Seconds())
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	m := s / 60
	s = s % 60
	if m < 60 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := m / 60
	m = m % 60
	return fmt.Sprintf("%dh %dm", h, m)
}

func printStatus(dir string, cfg *config.Config, clearEOL bool) error {
	// When clearEOL is true (follow mode), append ANSI erase-to-end-of-line
	// after each line to prevent stale characters from shorter redraws.
	eol := "\n"
	if clearEOL {
		eol = "\033[K\n"
	}

	// Pre-compute watched branch info and station distances for the
	// commit-distance indicator column.
	watchedRef, _ := git.HeadShortRef(dir)
	watchedDirty, _ := git.IsDirty(dir)
	watchedFullRef, _ := git.Run(dir, "rev-parse", cfg.Settings.Watches)

	type stationDist struct {
		ahead, behind int
		exists        bool
	}
	dists := make([]stationDist, len(cfg.Stations))
	maxBehind := 0
	if watchedFullRef != "" {
		for i, station := range cfg.Stations {
			branchName := git.StationBranchName(station.Name)
			if git.BranchExists(dir, branchName) {
				ahead, behind, err := git.RevDistance(dir, watchedFullRef, branchName)
				if err == nil {
					dists[i] = stationDist{ahead: ahead, behind: behind, exists: true}
					if behind > maxBehind {
						maxBehind = behind
					}
				}
			}
		}
	}

	// Build indicator strings: master shows dashes-then-H, stations show
	// their position relative to HEAD.
	masterInd := strings.Repeat("-", maxBehind) + "H"
	stnInds := make([]string, len(cfg.Stations))
	for i, d := range dists {
		switch {
		case !d.exists:
			stnInds[i] = ""
		case d.behind > 0:
			stnInds[i] = strings.Repeat("-", d.behind)
		case d.ahead > 0:
			stnInds[i] = "H" + strings.Repeat("+", d.ahead)
		default:
			stnInds[i] = "H"
		}
	}

	// Indicator column width = longest indicator + 1 space padding.
	indW := len(masterInd)
	for _, s := range stnInds {
		if len(s) > indW {
			indW = len(s)
		}
	}
	indW++ // trailing space

	// STAT-3: Line runner indicator at the top
	pid, _ := state.ReadPID(dir)
	configName := filepath.Base(configPath)
	if pid > 0 && state.IsProcessRunning(pid) {
		fmt.Fprintf(os.Stdout, "%s▶%s %s%s", colorGreen, colorReset, configName, eol)
	} else {
		fmt.Fprintf(os.Stdout, "%s⏸%s %s%s", colorGrey, colorReset, configName, eol)
	}

	// Blank line + column headers (indicator column has no header)
	fmt.Fprintf(os.Stdout, "%s", eol)
	fmt.Fprintf(os.Stdout, "%-21s%-*s%-9s%s%s", "Stations", indW, "", "Head", "Status", eol)

	// Print watched branch
	dirtyStr := ""
	if watchedDirty {
		dirtyStr = "(dirty)"
	}
	fmt.Fprintf(os.Stdout, "%-21s%-*s%-9s%s%s", cfg.Settings.Watches, indW, masterInd, watchedRef, dirtyStr, eol)

	// Print each station, tracking the first running station for log display
	var runningStation string
	for i, station := range cfg.Stations {
		branchName := git.StationBranchName(station.Name)
		ref := "-"

		if git.BranchExists(dir, branchName) {
			if branchRef, err := git.Run(dir, "rev-parse", "--short", branchName); err == nil {
				ref = branchRef
			}
		}

		info := computeStationInfo(dir, station, watchedFullRef, cfg.Settings.Watches)
		extra := ""
		if !info.startTime.IsZero() {
			// STAT-7: Show uptime duration instead of PID/start time
			extra = fmt.Sprintf(" (%s)", formatUptime(info.startTime))
			if runningStation == "" {
				runningStation = station.Name
			}
		}

		fmt.Fprintf(os.Stdout, "%s  %s %-17s%-*s%-9s[%s]%s%s%s", info.color, info.symbol, station.Name, indW, stnInds[i], ref, info.name, extra, colorReset, eol)
	}

	// In follow mode, show last lines of the running agent's output.
	// The log window height is dynamically sized to fill the terminal.
	if clearEOL && runningStation != "" {
		// Fixed rows: runner indicator + blank + column headers + watched branch
		//             + stations + blank separator + log header + 1 trailing
		//             newline (prevents the last \n from scrolling the terminal)
		fixedRows := 7 + len(cfg.Stations)
		logLines, termWidth := logWindowSize(fixedRows)
		if !printAgentLog(dir, runningStation, eol, logLines, termWidth) && !tmux.Available() {
			fmt.Fprintf(os.Stdout, "%s%sInstall tmux to see streaming agent output%s%s", eol, colorGrey, colorReset, eol)
		}
	}

	return nil
}

// ansiRE matches ANSI escape sequences (CSI, OSC, and single-char escapes).
var ansiRE = regexp.MustCompile(`\x1b(?:\[[0-9;?]*[a-zA-Z]|\][^\x07]*\x07|\[[^\x1b]*|.)`)

// stripANSI removes ANSI escape sequences and carriage returns from s.
func stripANSI(s string) string {
	s = ansiRE.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// logWindowSize returns the number of lines available for the log window and
// the terminal width. Lines are truncated to the terminal width to prevent
// wrapping from blowing past the calculated height.
func logWindowSize(fixedRows int) (lines, width int) {
	const fallbackLines = 15
	const fallbackWidth = 0 // 0 means no truncation
	const minLines = 3
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || h <= 0 {
		return fallbackLines, fallbackWidth
	}
	available := h - fixedRows
	if available < minLines {
		available = minLines
	}
	return available, w
}

// printAgentLog prints the last non-blank lines of agent output for the given station.
// Prefers tmux capture-pane (clean rendered pane content, live in interactive mode)
// over the raw pipe-pane log file (which contains ANSI escape sequences).
// Lines are truncated to termWidth to prevent wrapping (0 means no truncation).
func printAgentLog(dir, stationName, eol string, lineCount, termWidth int) bool {
	var lines []string

	// Prefer capture-pane: in interactive mode (no -p), Claude Code streams
	// output to the pane and capture-pane shows the live rendered screen.
	if sessionName := state.ReadStationTmux(dir, stationName); sessionName != "" && tmux.Available() {
		if captured, err := tmux.CapturePaneLines(sessionName, lineCount); err == nil {
			trimmed := stripTUIChrome(trimBlankLines(strings.Split(captured, "\n")))
			if len(trimmed) > lineCount {
				trimmed = trimmed[len(trimmed)-lineCount:]
			}
			lines = trimmed
		}
	}

	// Fall back to pipe-pane log file (strip ANSI escape sequences)
	if len(lines) == 0 {
		logPath := state.StationLogPath(dir, stationName)
		if data, err := os.ReadFile(logPath); err == nil && len(data) > 0 {
			cleaned := stripANSI(string(data))
			all := trimBlankLines(strings.Split(cleaned, "\n"))
			start := 0
			if len(all) > lineCount {
				start = len(all) - lineCount
			}
			lines = all[start:]
		}
	}

	if len(lines) == 0 {
		return false
	}

	// Print separator and output in grey, truncating lines to terminal width
	fmt.Fprintf(os.Stdout, "%s", eol)
	fmt.Fprintf(os.Stdout, "%s--- %s ---%s%s", colorGrey, stationName, colorReset, eol)
	for _, line := range lines {
		fmt.Fprintf(os.Stdout, "%s%s%s%s", colorGrey, truncateLine(line, termWidth), colorReset, eol)
	}
	return true
}

// isSeparatorLine returns true if a line consists entirely of box-drawing
// horizontal characters (─, U+2500), indicating a Claude Code TUI separator.
func isSeparatorLine(line string) bool {
	if line == "" {
		return false
	}
	for _, r := range line {
		if r != '─' {
			return false
		}
	}
	return true
}

// stripTUIChrome removes Claude Code's TUI chrome (separator lines, prompt,
// status bar) from capture-pane output. The chrome appears at the bottom of
// the pane after the last content line, starting with a full-width ─ separator.
func stripTUIChrome(lines []string) []string {
	// Find the last separator line — everything from there onward is chrome.
	cutoff := len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		if isSeparatorLine(strings.TrimSpace(lines[i])) {
			cutoff = i
		} else if cutoff < len(lines) {
			// We've passed back through chrome into content — stop.
			break
		}
	}
	if cutoff == 0 {
		return lines // don't strip everything
	}
	return lines[:cutoff]
}

// truncateLine truncates a line to fit within maxWidth columns.
// Returns the line unchanged if maxWidth is 0 (no truncation) or the line fits.
func truncateLine(line string, maxWidth int) string {
	if maxWidth <= 0 {
		return line
	}
	runes := []rune(line)
	if len(runes) <= maxWidth {
		return line
	}
	return string(runes[:maxWidth])
}

// trimBlankLines removes leading and trailing blank lines from a slice.
func trimBlankLines(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	if start >= end {
		return nil
	}
	return lines[start:end]
}

func init() {
	statusCmd.Flags().BoolVarP(&followFlag, "follow", "f", false, "refresh every 2 seconds")
	rootCmd.AddCommand(statusCmd)
}
