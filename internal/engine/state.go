package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/re-cinq/detergent/internal/fileutil"
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
	return fileutil.DetergentSubdir(repoDir, "state")
}

// stateFilePath returns the full path to a concern's state file.
func stateFilePath(repoDir, concernName string) string {
	return filepath.Join(stateDir(repoDir), concernName)
}

// LastSeen returns the last-seen commit hash for a concern, or "" if none.
func LastSeen(repoDir, concernName string) (string, error) {
	path := stateFilePath(repoDir, concernName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading last-seen for %s: %w", concernName, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// ConcernStatus represents the current lifecycle state of a concern.
type ConcernStatus struct {
	State       string `json:"state"`                   // idle, change_detected, agent_running, committing, failed, skipped
	LastResult  string `json:"last_result,omitempty"`   // noop, modified
	StartedAt   string `json:"started_at,omitempty"`    // RFC3339
	CompletedAt string `json:"completed_at,omitempty"`  // RFC3339
	Error       string `json:"error,omitempty"`         // error message if failed
	LastSeen    string `json:"last_seen,omitempty"`     // last processed commit hash
	HeadAtStart string `json:"head_at_start,omitempty"` // HEAD when processing started
	PID         int    `json:"pid"`                     // process ID
}

// statusDir returns the status directory path for a repo.
func statusDir(repoDir string) string {
	return fileutil.DetergentSubdir(repoDir, "status")
}

// statusFilePath returns the full path to a concern's status JSON file.
func statusFilePath(repoDir, concernName string) string {
	return filepath.Join(statusDir(repoDir), concernName+".json")
}

// WriteStatus writes a concern's status to its JSON status file.
func WriteStatus(repoDir, concernName string, status *ConcernStatus) error {
	dir := statusDir(repoDir)
	if err := fileutil.EnsureDir(dir); err != nil {
		return err
	}
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return os.WriteFile(statusFilePath(repoDir, concernName), data, 0644)
}

// ReadStatus reads a concern's status from its JSON status file.
func ReadStatus(repoDir, concernName string) (*ConcernStatus, error) {
	path := statusFilePath(repoDir, concernName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading status for %s: %w", concernName, err)
	}
	var status ConcernStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("parsing status for %s: %w", concernName, err)
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

// ResetActiveStatuses resets any concern status that is in an active state
// (change_detected, agent_running, committing) back to idle. This should be
// called at the start of each processing cycle — any active status at that
// point is stale from a previous run that was interrupted (e.g., daemon killed).
func ResetActiveStatuses(repoDir string, concernNames []string) {
	pid := os.Getpid()
	for _, name := range concernNames {
		status, err := ReadStatus(repoDir, name)
		if err != nil || status == nil {
			continue
		}
		if !IsActiveState(status.State) {
			continue
		}
		_ = WriteStatus(repoDir, name, &ConcernStatus{
			State:      StateFailed,
			Error:      fmt.Sprintf("stale %s state cleared on startup (previous process interrupted)", status.State),
			LastSeen:   status.LastSeen,
			LastResult: status.LastResult,
			PID:        pid,
		})
	}
}

// SetLastSeen records the last-seen commit hash for a concern.
func SetLastSeen(repoDir, concernName, hash string) error {
	dir := stateDir(repoDir)
	if err := fileutil.EnsureDir(dir); err != nil {
		return err
	}
	return os.WriteFile(stateFilePath(repoDir, concernName), []byte(hash+"\n"), 0644)
}
