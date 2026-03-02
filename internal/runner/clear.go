package runner

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/re-cinq/assembly-line/internal/config"
	"github.com/re-cinq/assembly-line/internal/git"
	"github.com/re-cinq/assembly-line/internal/state"
	"github.com/re-cinq/assembly-line/internal/tmux"
)

// Clear stops all running processes and resets all station state.
func Clear(dir string, cfg *config.Config) error {
	// 1. Kill all station agents first — they run in their own process groups
	state.KillAllStationAgents(dir)

	// 2. Kill runner process
	if pid, _ := state.ReadPID(dir); pid > 0 && state.IsProcessRunning(pid) {
		_ = state.KillProcessGroup(pid)
	}

	// 3. Clean any remaining tmux sessions
	if tmux.Available() {
		_ = tmux.CleanStaleSessions(dir)
	}

	// 4. Remove worktrees
	if baseDir, err := git.WorktreeBaseDir(dir); err == nil {
		_ = os.RemoveAll(baseDir)
	}
	_ = git.PruneWorktrees(dir)

	// 5. Delete station branches
	for _, station := range cfg.Stations {
		_ = git.DeleteBranch(dir, git.StationBranchName(station.Name))
	}

	// 6. Remove .line/stations/ directory
	_ = os.RemoveAll(filepath.Join(dir, ".line", "stations"))

	// 7. Remove .line/run.pid
	_ = state.RemovePID(dir)

	fmt.Println("assembly-line cleared")
	return nil
}
