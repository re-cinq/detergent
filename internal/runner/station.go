package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/re-cinq/assembly-line/internal/config"
	"github.com/re-cinq/assembly-line/internal/git"
	"github.com/re-cinq/assembly-line/internal/settings"
	"github.com/re-cinq/assembly-line/internal/state"
)

// runStation executes a single station in an ephemeral git worktree (RUN-15).
// The user's working tree is never disturbed.
func runStation(dir string, cfg *config.Config, station config.Station, predecessor string) error {
	resolved := cfg.ResolveStation(station)
	branchName := git.StationBranchName(station.Name)

	// Create branch if it doesn't exist (RUN-6: catch up)
	if !git.BranchExists(dir, branchName) {
		if err := git.CreateBranch(dir, branchName, predecessor); err != nil {
			return fmt.Errorf("creating branch %s: %w", branchName, err)
		}
	}

	// Compute worktree path (RUN-15)
	baseDir, err := git.WorktreeBaseDir(dir)
	if err != nil {
		return fmt.Errorf("station %s: worktree base dir: %w", station.Name, err)
	}
	wtPath := filepath.Join(baseDir, station.Name)

	// Clean up any leftover worktree at that path (crash recovery)
	_ = git.RemoveWorktree(dir, wtPath)
	_ = os.RemoveAll(wtPath)

	// Create the worktree
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return fmt.Errorf("station %s: creating worktree base dir: %w", station.Name, err)
	}
	if err := git.AddWorktree(dir, wtPath, branchName); err != nil {
		return fmt.Errorf("station %s: adding worktree: %w", station.Name, err)
	}
	defer func() {
		_ = git.RemoveWorktree(dir, wtPath)
		_ = os.RemoveAll(wtPath)
	}()

	// Rebase onto predecessor to pick up changes (in the worktree)
	if err := git.Rebase(wtPath, predecessor); err != nil {
		// RUN-6: If rebase fails, reset to predecessor and try again
		fmt.Fprintf(os.Stderr, "station %s: rebase conflict, resetting to %s\n", station.Name, predecessor)
		_ = git.RebaseAbort(wtPath)
		if err := git.ResetHard(wtPath, predecessor); err != nil {
			return fmt.Errorf("station %s: reset failed: %w", station.Name, err)
		}
	}

	// Run the agent in the worktree (RUN-1, RUN-12)
	agent, err := startAgent(wtPath, resolved.Command, resolved.Args, resolved.Prompt, station.Name, dir)
	if err != nil {
		return fmt.Errorf("station %s: %w", station.Name, err)
	}

	// Write station PID file in main repo so status can detect the running agent
	_ = state.WriteStationPID(dir, station.Name, agent.pid(), time.Now())

	// Write tmux session name if running in tmux
	if agent.session() != "" {
		_ = state.WriteStationTmux(dir, station.Name, agent.session())
	}

	// Wait for agent to complete
	agentErr := agent.wait()

	// Remove .claude/ from the worktree — ConfigureAgentDoneHook created
	// settings.json there and it should not be committed to the station branch.
	_ = os.RemoveAll(filepath.Join(wtPath, ".claude"))

	// Claude Code syncs worktree settings to the main repo, so the agent
	// done hook (touch .line-agent-done) can leak into the main repo's
	// .claude/settings.json. Clean it up and restore auto-rebase hooks.
	// Only if the file already exists — don't create it from scratch.
	if _, statErr := os.Stat(filepath.Join(dir, ".claude", "settings.json")); statErr == nil {
		_ = settings.RemoveAgentDoneHooks(dir)
		_ = settings.ConfigureAutoRebaseHook(dir)
	}

	// Clean up station state files
	_ = state.RemoveStationPID(dir, station.Name)
	_ = state.RemoveStationTmux(dir, station.Name)

	// RUN-14: A failed station blocks the line and is reported as 'failed'
	if agentErr != nil {
		fmt.Fprintf(os.Stderr, "station %s: agent exited with error: %v\n", station.Name, agentErr)
		_ = state.WriteStationFailed(dir, station.Name)
		return fmt.Errorf("agent failed: %w", agentErr)
	}
	_ = state.RemoveStationFailed(dir, station.Name)

	// RUN-5: Commit any changes with skip marker (RUN-4, RUN-9)
	commitMsg := fmt.Sprintf("assembly-line: station %s %s", station.Name, commitSkipMarker)
	if err := git.CommitAll(wtPath, commitMsg); err != nil {
		fmt.Fprintf(os.Stderr, "station %s: commit failed: %v\n", station.Name, err)
	}

	return nil
}
