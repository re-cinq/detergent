package runner

import (
	"fmt"
	"os"
	"strings"

	"github.com/re-cinq/assembly-line/internal/config"
	"github.com/re-cinq/assembly-line/internal/git"
	"github.com/re-cinq/assembly-line/internal/ignore"
	"github.com/re-cinq/assembly-line/internal/state"
	"github.com/re-cinq/assembly-line/internal/tmux"
)

// commitSkipMarker is appended to station commit messages. It must be one of
// SkipMarkers so that Run() does not retrigger on station commits.
const commitSkipMarker = "[skip line]"

// SkipMarkers are commit message markers that prevent retriggering.
var SkipMarkers = []string{"[skip ci]", "[ci skip]", commitSkipMarker, "[line skip]"}

// Run executes the full assembly line pipeline.
func Run(dir string, cfg *config.Config) error {
	// RUN-4 layer 2: Check env var guard
	if os.Getenv("LINE_RUNNING") == "1" {
		fmt.Fprintln(os.Stderr, "assembly-line: skipping (LINE_RUNNING=1)")
		return nil
	}

	// RUN-4 layer 1: Check if we're on the watched branch
	currentBranch, err := git.CurrentBranch(dir)
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}
	if currentBranch != cfg.Settings.Watches {
		fmt.Fprintf(os.Stderr, "assembly-line: skipping (not on watched branch %s, on %s)\n", cfg.Settings.Watches, currentBranch)
		return nil
	}

	// RUN-9: Check if the last commit message contains a skip marker
	lastMsg, err := git.LastCommitMessage(dir)
	if err != nil {
		return fmt.Errorf("getting last commit message: %w", err)
	}
	for _, marker := range SkipMarkers {
		if strings.Contains(lastMsg, marker) {
			fmt.Fprintf(os.Stderr, "assembly-line: skipping (commit contains %s)\n", marker)
			return nil
		}
	}

	// RUN-7, RUN-8: Check .lineignore
	parentRef := "HEAD~1"
	changedFiles, _ := git.DiffFiles(dir, parentRef, "HEAD")
	if len(changedFiles) > 0 {
		matcher, err := ignore.Load(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "assembly-line: warning: could not load .lineignore: %v\n", err)
		} else if matcher.AllIgnored(changedFiles) {
			fmt.Fprintln(os.Stderr, "assembly-line: skipping (all changed files are ignored)")
			return nil
		}
	}

	// RUN-11: Check for existing runner and terminate it
	existingPID, err := state.ReadPID(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "assembly-line: warning: could not read PID: %v\n", err)
	}
	if existingPID > 0 && state.IsProcessRunning(existingPID) {
		fmt.Fprintf(os.Stderr, "assembly-line: terminating previous run (PID %d)\n", existingPID)
		// Kill station agents first — they run in their own process groups
		// (Setpgid) so killing the runner alone won't reach them.
		state.KillAllStationAgents(dir)
		if err := state.KillProcessGroup(existingPID); err != nil {
			fmt.Fprintf(os.Stderr, "assembly-line: warning: could not kill previous run: %v\n", err)
		}
	}

	// Write our PID
	if err := state.WritePID(dir, os.Getpid()); err != nil {
		return fmt.Errorf("writing PID: %w", err)
	}
	defer func() { _ = state.RemovePID(dir) }()

	// Set env var to prevent retriggering
	os.Setenv("LINE_RUNNING", "1")
	defer os.Unsetenv("LINE_RUNNING")

	// Clean up stale tmux sessions from previous runs
	if tmux.Available() {
		_ = tmux.CleanStaleSessions(dir)
	}

	// RUN-15: Clean up stale worktrees from previous runs and after this run.
	// Remove directories first so that prune sees them as gone and cleans
	// up the git bookkeeping entries.
	if baseDir, err := git.WorktreeBaseDir(dir); err == nil {
		_ = os.RemoveAll(baseDir)
		defer os.RemoveAll(baseDir)
	}
	_ = git.PruneWorktrees(dir)

	// RUN-1: Execute stations in sequence
	// The chain: watched_branch -> station1 -> station2 -> ... -> stationN
	predecessor := cfg.Settings.Watches
	for _, station := range cfg.Stations {
		fmt.Fprintf(os.Stderr, "assembly-line: running station %s\n", station.Name)
		if err := runStation(dir, cfg, station, predecessor); err != nil {
			fmt.Fprintf(os.Stderr, "assembly-line: station %s failed: %v\n", station.Name, err)
			break
		}
		predecessor = git.StationBranchName(station.Name)
	}

	return nil
}
