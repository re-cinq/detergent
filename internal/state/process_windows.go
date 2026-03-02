//go:build windows

package state

import "os"

// IsProcessRunning checks if a process with the given PID is running.
func IsProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := findProcess(pid)
	if err != nil {
		return false
	}
	// On Windows, FindProcess always succeeds; Signal(0) is not supported.
	// Sending os.Kill and checking for error is the standard approach.
	err = process.Signal(os.Kill)
	return err == nil
}

// KillProcessGroup kills the process with the given PID.
// Windows does not have process groups, so this kills the process directly.
func KillProcessGroup(pid int) error {
	if pid <= 0 {
		return nil
	}
	process, err := findProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

// KillTmuxSession is a no-op on Windows (tmux is unavailable).
func KillTmuxSession(sessionName string) {}
