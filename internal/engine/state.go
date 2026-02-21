package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/re-cinq/assembly-line/internal/fileutil"
)

// State constants
const (
	StateIdle           = "idle"
	StateChangeDetected = "change_detected"
	StateAgentRunning   = "agent_running"
	StateCommitting     = "committing"
	StateFailed         = "failed"
	StateSkipped        = "skipped"
)

// Result constants
const (
	ResultNoop     = "noop"
	ResultModified = "modified"
)

// stateDir returns the state directory path for a repo.
func stateDir(repoDir string) string {
	return fileutil.LineSubdir(repoDir, "state")
}

// stateFilePath returns the full path to a station's state file.
func stateFilePath(repoDir, stationName string) string {
	return filepath.Join(stateDir(repoDir), stationName)
}

// LastSeen returns the last-seen commit hash for a station, or "" if none.
func LastSeen(repoDir, stationName string) (string, error) {
	path := stateFilePath(repoDir, stationName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading last-seen for %s: %w", stationName, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// StationStatus represents the current lifecycle state of a station.
type StationStatus struct {
	State       string `json:"state"`                   // idle, change_detected, agent_running, committing, failed, skipped
	LastResult  string `json:"last_result,omitempty"`   // noop, modified
	StartedAt   string `json:"started_at,omitempty"`    // RFC3339
	CompletedAt string `json:"completed_at,omitempty"`  // RFC3339
	Error       string `json:"error,omitempty"`         // error message if failed
	HeadAtStart string `json:"head_at_start,omitempty"` // HEAD when processing started
	PID         int    `json:"pid"`                     // process ID
}

// statusDir returns the status directory path for a repo.
func statusDir(repoDir string) string {
	return fileutil.LineSubdir(repoDir, "status")
}

// statusFilePath returns the full path to a station's status JSON file.
func statusFilePath(repoDir, stationName string) string {
	return filepath.Join(statusDir(repoDir), stationName+".json")
}

// WriteStatus writes a station's status to its JSON status file.
func WriteStatus(repoDir, stationName string, status *StationStatus) error {
	dir := statusDir(repoDir)
	if err := fileutil.EnsureDir(dir); err != nil {
		return err
	}
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return os.WriteFile(statusFilePath(repoDir, stationName), data, 0644)
}

// ReadStatus reads a station's status from its JSON status file.
func ReadStatus(repoDir, stationName string) (*StationStatus, error) {
	path := statusFilePath(repoDir, stationName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading status for %s: %w", stationName, err)
	}
	var status StationStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("parsing status for %s: %w", stationName, err)
	}
	return &status, nil
}

// nowRFC3339 returns the current time in RFC3339 format.
func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// IsActiveState returns true if the state represents an in-progress operation.
func IsActiveState(state string) bool {
	switch state {
	case StateChangeDetected, StateAgentRunning, StateCommitting:
		return true
	}
	return false
}

// IsProcessAlive checks if a process with the given PID is still running.
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// ResetActiveStatuses resets any station status that is in an active state
// (change_detected, agent_running, committing) back to idle. This should be
// called at the start of each processing cycle — any active status at that
// point is stale from a previous run that was interrupted (e.g., daemon killed).
func ResetActiveStatuses(repoDir string, stationNames []string) {
	pid := os.Getpid()
	for _, name := range stationNames {
		status, err := ReadStatus(repoDir, name)
		if err != nil || status == nil {
			continue
		}
		if !IsActiveState(status.State) {
			continue
		}
		writeStaleFailedStatus(repoDir, name, status.State, status.LastResult, pid)
	}
}

// SetLastSeen records the last-seen commit hash for a station.
func SetLastSeen(repoDir, stationName, hash string) error {
	dir := stateDir(repoDir)
	if err := fileutil.EnsureDir(dir); err != nil {
		return err
	}
	return os.WriteFile(stateFilePath(repoDir, stationName), []byte(hash+"\n"), 0644)
}

// writeStaleFailedStatus writes a failed status for a stale active state that was interrupted.
// This is called on startup when we find a station stuck in an active state from a previous run.
func writeStaleFailedStatus(repoDir, stationName, staleState, lastResult string, pid int) {
	writeStatus(repoDir, stationName, statusUpdate{
		state:      StateFailed,
		errorMsg:   fmt.Sprintf("stale %s state cleared on startup (previous process interrupted)", staleState),
		lastResult: lastResult,
		pid:        pid,
	})
}
