package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
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
	return filepath.Join(repoDir, ".detergent", "state")
}

// LastSeen returns the last-seen commit hash for a concern, or "" if none.
func LastSeen(repoDir, concernName string) (string, error) {
	path := filepath.Join(stateDir(repoDir), concernName)
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
	State       string `json:"state"`                   // running, idle, failed, skipped
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
	return filepath.Join(repoDir, ".detergent", "status")
}

// WriteStatus writes a concern's status to its JSON status file.
func WriteStatus(repoDir, concernName string, status *ConcernStatus) error {
	dir := statusDir(repoDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, concernName+".json"), data, 0644)
}

// ReadStatus reads a concern's status from its JSON status file.
func ReadStatus(repoDir, concernName string) (*ConcernStatus, error) {
	path := filepath.Join(statusDir(repoDir), concernName+".json")
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
	case StateChangeDetected, StateAgentRunning, StateCommitting, "running":
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

// SetLastSeen records the last-seen commit hash for a concern.
func SetLastSeen(repoDir, concernName, hash string) error {
	dir := stateDir(repoDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, concernName), []byte(hash+"\n"), 0644)
}
