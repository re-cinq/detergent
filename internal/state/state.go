package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	stateDir            = ".line"
	pidFile             = "run.pid"
	rebasePromptedFile  = "rebase-prompted"
	stationsDir         = "stations"
)

// ensureDir creates the .line directory if it doesn't exist.
func ensureDir(repoDir string) error {
	dir := filepath.Join(repoDir, stateDir)
	return os.MkdirAll(dir, 0o755)
}

// ensureStationsDir creates the .line/stations directory.
func ensureStationsDir(repoDir string) error {
	return os.MkdirAll(filepath.Join(repoDir, stateDir, stationsDir), 0o755)
}

// stationFilePath returns the full path for a station's state file.
func stationFilePath(repoDir, stationName, suffix string) string {
	return filepath.Join(repoDir, stateDir, stationsDir, stationName+suffix)
}

// removeFile removes a file, returning nil if it doesn't exist.
func removeFile(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// WritePID writes the runner PID file.
func WritePID(repoDir string, pid int) error {
	if err := ensureDir(repoDir); err != nil {
		return err
	}
	path := filepath.Join(repoDir, stateDir, pidFile)
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o644)
}

// ReadPID reads the runner PID. Returns 0 if no PID file exists.
func ReadPID(repoDir string) (int, error) {
	path := filepath.Join(repoDir, stateDir, pidFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(string(data))
}

// RemovePID removes the PID file.
func RemovePID(repoDir string) error {
	return removeFile(filepath.Join(repoDir, stateDir, pidFile))
}

// WriteRebasePrompted records the terminal ref that was last auto-rebased.
func WriteRebasePrompted(repoDir, ref string) error {
	if err := ensureDir(repoDir); err != nil {
		return err
	}
	path := filepath.Join(repoDir, stateDir, rebasePromptedFile)
	return os.WriteFile(path, []byte(ref), 0o644)
}

// ReadRebasePrompted returns the stored terminal ref, or "" if none exists.
func ReadRebasePrompted(repoDir string) string {
	path := filepath.Join(repoDir, stateDir, rebasePromptedFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// RemoveRebasePrompted removes the rebase-prompted marker.
func RemoveRebasePrompted(repoDir string) error {
	return removeFile(filepath.Join(repoDir, stateDir, rebasePromptedFile))
}

// findProcess wraps os.FindProcess for use in platform-specific code.
func findProcess(pid int) (*os.Process, error) {
	return os.FindProcess(pid)
}

// WriteStationPID writes a station's agent PID and start time.
// Format: "PID TIMESTAMP" (e.g., "12345 2024-01-15T10:30:00Z")
func WriteStationPID(repoDir, stationName string, pid int, startTime time.Time) error {
	if err := ensureStationsDir(repoDir); err != nil {
		return err
	}
	content := fmt.Sprintf("%d %s", pid, startTime.Format(time.RFC3339))
	return os.WriteFile(stationFilePath(repoDir, stationName, ".pid"), []byte(content), 0o644)
}

// ReadStationPID reads a station's agent PID and start time.
// Returns pid=0 if no PID file exists.
func ReadStationPID(repoDir, stationName string) (int, time.Time, error) {
	path := stationFilePath(repoDir, stationName, ".pid")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0, time.Time{}, nil
	}
	if err != nil {
		return 0, time.Time{}, err
	}
	parts := strings.SplitN(strings.TrimSpace(string(data)), " ", 2)
	pid, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("parsing station PID: %w", err)
	}
	var startTime time.Time
	if len(parts) > 1 {
		startTime, _ = time.Parse(time.RFC3339, parts[1])
	}
	return pid, startTime, nil
}

// RemoveStationPID removes a station's PID file.
func RemoveStationPID(repoDir, stationName string) error {
	return removeFile(stationFilePath(repoDir, stationName, ".pid"))
}

// KillAllStationAgents kills all running station agent processes and removes
// their PID and tmux state files. If a station has a .tmux file, the tmux
// session is killed instead of relying on the process group alone.
func KillAllStationAgents(repoDir string) {
	dir := filepath.Join(repoDir, stateDir, stationsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".pid") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".pid")

		// Prefer tmux session kill if a .tmux state file exists
		if sessionName := ReadStationTmux(repoDir, name); sessionName != "" {
			KillTmuxSession(sessionName)
			_ = RemoveStationTmux(repoDir, name)
			_ = RemoveStationPID(repoDir, name)
			continue
		}

		pid, _, _ := ReadStationPID(repoDir, name)
		if pid > 0 && IsProcessRunning(pid) {
			_ = KillProcessGroup(pid)
		}
		_ = RemoveStationPID(repoDir, name)
	}
}

// WriteStationFailed writes a marker indicating a station's agent failed.
func WriteStationFailed(repoDir, stationName string) error {
	if err := ensureStationsDir(repoDir); err != nil {
		return err
	}
	return os.WriteFile(stationFilePath(repoDir, stationName, ".failed"), []byte("1"), 0o644)
}

// ReadStationFailed returns true if a station has a failure marker.
func ReadStationFailed(repoDir, stationName string) bool {
	_, err := os.Stat(stationFilePath(repoDir, stationName, ".failed"))
	return err == nil
}

// RemoveStationFailed removes a station's failure marker.
func RemoveStationFailed(repoDir, stationName string) error {
	return removeFile(stationFilePath(repoDir, stationName, ".failed"))
}

// StationLogPath returns the path to a station's tmux pipe-pane log file.
func StationLogPath(repoDir, stationName string) string {
	return stationFilePath(repoDir, stationName, ".log")
}

// WriteStationTmux writes the tmux session name for a running station.
func WriteStationTmux(repoDir, stationName, sessionName string) error {
	if err := ensureStationsDir(repoDir); err != nil {
		return err
	}
	return os.WriteFile(stationFilePath(repoDir, stationName, ".tmux"), []byte(sessionName), 0o644)
}

// ReadStationTmux reads the tmux session name for a station.
// Returns "" if no tmux state file exists.
func ReadStationTmux(repoDir, stationName string) string {
	data, err := os.ReadFile(stationFilePath(repoDir, stationName, ".tmux"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// RemoveStationTmux removes a station's tmux state file.
func RemoveStationTmux(repoDir, stationName string) error {
	return removeFile(stationFilePath(repoDir, stationName, ".tmux"))
}
