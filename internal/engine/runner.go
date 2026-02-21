package engine

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/re-cinq/assembly-line/internal/config"
	"github.com/re-cinq/assembly-line/internal/fileutil"
)

// GracePeriod is how long the runner waits for new work before exiting.
var GracePeriod = 5 * time.Second

// TriggerPath returns the path to the trigger file for a repo.
func TriggerPath(repoDir string) string {
	return fileutil.LineSubdir(repoDir, "trigger")
}

// WriteTrigger writes a commit hash to the trigger file.
func WriteTrigger(repoDir, commitHash string) error {
	if err := ensureLineDotDir(repoDir); err != nil {
		return err
	}
	return os.WriteFile(TriggerPath(repoDir), []byte(commitHash+"\n"), 0644)
}

// ReadTrigger reads the trigger file, returning the hash and modification time.
// Returns empty hash and zero time (no error) if the file does not exist.
func ReadTrigger(repoDir string) (hash string, modTime time.Time, err error) {
	path := TriggerPath(repoDir)
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, fmt.Errorf("stat trigger file: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("reading trigger file: %w", err)
	}
	return strings.TrimSpace(string(data)), info.ModTime(), nil
}

// PIDPath returns the path to the runner PID file for a repo.
func PIDPath(repoDir string) string {
	return fileutil.LineSubdir(repoDir, "runner.pid")
}

// WritePID writes the current process ID to the PID file.
func WritePID(repoDir string) error {
	if err := ensureLineDotDir(repoDir); err != nil {
		return err
	}
	return os.WriteFile(PIDPath(repoDir), []byte(strconv.Itoa(os.Getpid())+"\n"), 0644)
}

// ensureLineDotDir ensures the .line directory exists in the repository.
func ensureLineDotDir(repoDir string) error {
	dir := fileutil.LineDir(repoDir)
	if err := fileutil.EnsureDir(dir); err != nil {
		return fmt.Errorf("creating .line directory: %w", err)
	}
	return nil
}

// ReadPID reads the PID from the PID file. Returns 0 on any error.
func ReadPID(repoDir string) int {
	data, err := os.ReadFile(PIDPath(repoDir))
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

// RemovePID removes the PID file, ignoring errors.
func RemovePID(repoDir string) {
	os.Remove(PIDPath(repoDir))
}

// IsRunnerAlive checks if a runner process is alive by reading the PID file
// and checking the process.
func IsRunnerAlive(repoDir string) bool {
	pid := ReadPID(repoDir)
	return IsProcessAlive(pid)
}

// reloadRunnerConfig attempts to reload and validate the config file.
// On any error, the previous config is returned unchanged.
func reloadRunnerConfig(configPath string, prev *config.Config) *config.Config {
	newCfg, err := config.Load(configPath)
	if err != nil {
		fileutil.LogError("config reload: %s (keeping previous config)", err)
		return prev
	}
	if errs := config.Validate(newCfg); len(errs) > 0 {
		fileutil.LogError("config reload: invalid (%s) (keeping previous config)", errs[0])
		return prev
	}
	return newCfg
}

// RunnerLoop is the self-retiring runner loop. It processes stations, then waits
// one grace period for new trigger activity. If no new trigger arrives, it exits.
func RunnerLoop(ctx context.Context, configPath string, cfg *config.Config, repoDir string) error {
	// Duplicate guard: if another runner is already alive, exit immediately
	if IsRunnerAlive(repoDir) {
		fmt.Println("line runner already active, exiting")
		return nil
	}

	// Write PID file, clean up on exit
	if err := WritePID(repoDir); err != nil {
		return fmt.Errorf("writing PID file: %w", err)
	}
	defer RemovePID(repoDir)

	logMgr := NewLogManager()
	defer logMgr.Close()

	fmt.Printf("line runner started (grace period %s)\n", GracePeriod)
	fmt.Printf("Agent logs: %s\n", LogPath())

	// Record trigger mod time before the first run
	_, lastTriggerTime, err := ReadTrigger(repoDir)
	if err != nil {
		return fmt.Errorf("reading initial trigger: %w", err)
	}

	for {
		// Hot-reload config each cycle
		cfg = reloadRunnerConfig(configPath, cfg)

		if err := RunOnceWithLogs(cfg, repoDir, logMgr); err != nil {
			fileutil.LogError("run error: %s", err)
		}

		// Wait the grace period, honoring context cancellation
		select {
		case <-ctx.Done():
			fmt.Println("line runner stopped (signal)")
			return nil
		case <-time.After(GracePeriod):
		}

		// Check if trigger file was updated during the grace period
		_, newTriggerTime, err := ReadTrigger(repoDir)
		if err != nil {
			fileutil.LogError("reading trigger: %s", err)
			fmt.Println("line runner exiting (trigger read error)")
			return nil
		}

		if !newTriggerTime.After(lastTriggerTime) {
			fmt.Println("line runner exiting (no new work)")
			return nil
		}

		// New work arrived — record the time and loop
		lastTriggerTime = newTriggerTime
	}
}
