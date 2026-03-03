package tmux

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var (
	availableOnce   sync.Once
	availableResult bool
)

// Available returns true if tmux is installed and in PATH.
// The result is cached after the first call.
func Available() bool {
	availableOnce.Do(func() {
		_, err := exec.LookPath("tmux")
		availableResult = err == nil
	})
	return availableResult
}

// repoTag returns an 8-char hex tag derived from the canonical repo path.
func repoTag(repoDir string) string {
	abs, err := filepath.Abs(repoDir)
	if err != nil {
		abs = repoDir
	}
	// Resolve symlinks for a canonical path (e.g. /var -> /private/var on macOS)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	h := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(h[:])[:8]
}

// SessionName returns a deterministic tmux session name for a repo+station:
// line-<8-char-sha256-of-repo-path>-<stationName>
func SessionName(repoDir, stationName string) string {
	return "line-" + repoTag(repoDir) + "-" + stationName
}

// sessionPrefix returns the prefix shared by all sessions for a repo.
func sessionPrefix(repoDir string) string {
	return "line-" + repoTag(repoDir) + "-"
}

// NewSession creates a new detached tmux session running shellCmd in dir.
// remain-on-exit is set atomically via command chaining so it takes effect
// before the shell command can exit (avoids a race where fast-exiting
// commands destroy the pane before we can read exit status).
func NewSession(name, dir, shellCmd string) error {
	return run("new-session", "-d", "-s", name, "-c", dir, shellCmd,
		";", "set-option", "-t", name, "remain-on-exit", "on")
}

// HasSession returns true if the named tmux session exists.
func HasSession(name string) bool {
	err := run("has-session", "-t", "="+name)
	return err == nil
}

// KillSession kills the named tmux session.
// Returns nil if the session does not exist.
func KillSession(name string) error {
	if !HasSession(name) {
		return nil
	}
	return run("kill-session", "-t", "="+name)
}

// PanePID returns the PID of the process running in pane 0 of the session.
func PanePID(session string) (int, error) {
	out, err := output("display-message", "-t", session+":0.0", "-p", "#{pane_pid}")
	if err != nil {
		return 0, fmt.Errorf("getting pane PID: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("parsing pane PID %q: %w", out, err)
	}
	return pid, nil
}

// PaneStatus reads the pane dead status.
// Returns dead=true if the pane's process has exited, along with its exit code.
func PaneStatus(session string) (dead bool, exitCode int, err error) {
	out, err := output("display-message", "-t", session+":0.0", "-p", "#{pane_dead} #{pane_dead_status}")
	if err != nil {
		return false, 0, fmt.Errorf("getting pane status: %w", err)
	}
	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) == 0 {
		return false, 0, nil // no output — treat as alive
	}
	dead = parts[0] == "1"
	if len(parts) >= 2 {
		exitCode, _ = strconv.Atoi(parts[1])
	}
	return dead, exitCode, nil
}

// SendKeys sends a string to the session's pane followed by Enter.
func SendKeys(session, keys string) error {
	return run("send-keys", "-t", session, keys, "Enter")
}

// PipePane starts piping pane output to a shell command (typically "cat >> logfile").
func PipePane(session, shellCmd string) error {
	return run("pipe-pane", "-t", session, shellCmd)
}

// CapturePaneLines captures the last lineCount lines from the session's pane.
// Returns the rendered pane content (what you'd see on screen).
func CapturePaneLines(session string, lineCount int) (string, error) {
	out, err := output("capture-pane", "-t", session, "-p", "-S", fmt.Sprintf("-%d", lineCount))
	if err != nil {
		return "", fmt.Errorf("capturing pane: %w", err)
	}
	return out, nil
}

// CleanStaleSessions lists tmux sessions matching our prefix for the given
// repo and kills them.
func CleanStaleSessions(repoDir string) error {
	prefix := sessionPrefix(repoDir)
	out, err := output("list-sessions", "-F", "#{session_name}")
	if err != nil {
		// No server running or no sessions — nothing to clean
		return nil
	}
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if name != "" && strings.HasPrefix(name, prefix) {
			_ = run("kill-session", "-t", "="+name)
		}
	}
	return nil
}

// run executes a tmux command and returns any error.
func run(args ...string) error {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux %s: %s: %w", args[0], strings.TrimSpace(string(out)), err)
	}
	return nil
}

// output executes a tmux command and returns its stdout.
func output(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}
