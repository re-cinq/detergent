//go:build !windows

package state

import (
	"syscall"

	"github.com/re-cinq/assembly-line/internal/tmux"
)

// IsProcessRunning checks if a process with the given PID is running.
func IsProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := findProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// KillProcessGroup sends SIGTERM to the process group of the given PID.
// Falls back to killing the process directly if it is not a process group
// leader (e.g. when started from a git hook).
func KillProcessGroup(pid int) error {
	if pid <= 0 {
		return nil
	}
	// Try process group kill first (works if pid is a PGID)
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		// Process may not be a group leader (e.g. started from a hook
		// via "line run &"). Fall back to signaling the process directly.
		return syscall.Kill(pid, syscall.SIGTERM)
	}
	return nil
}

// KillTmuxSession kills the process inside a tmux session (via its pane PID
// process group) and then kills the session itself.
func KillTmuxSession(sessionName string) {
	if sessionName == "" || !tmux.Available() {
		return
	}
	if pid, err := tmux.PanePID(sessionName); err == nil && pid > 0 {
		_ = KillProcessGroup(pid)
	}
	_ = tmux.KillSession(sessionName)
}
