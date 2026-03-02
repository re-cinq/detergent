package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/re-cinq/assembly-line/internal/state"
	"github.com/re-cinq/assembly-line/internal/tmux"
)

const preamble = "IMPORTANT: Do NOT commit any changes. Do NOT run git commit. Make file changes only. The system will handle committing."

// agentProcess represents a running agent subprocess.
type agentProcess struct {
	cmd          *exec.Cmd // nil when using tmux path
	tmuxSession  string    // non-empty when running inside tmux
	tmuxPanePID  int       // PID of the process inside the tmux pane
	logPath      string    // path to pipe-pane log file
	stationName  string
	repoDir      string
	isClaudeCode bool      // true when the agent command is Claude Code
}

// startAgent launches an agent subprocess with the given command, args, and prompt.
// If tmux is available, the agent runs inside a tmux session for observability.
// Otherwise it falls back to direct subprocess execution.
// RUN-12: The preamble is prepended to the prompt.
func startAgent(dir, command string, args []string, prompt, stationName, repoDir string) (*agentProcess, error) {
	if tmux.Available() && stationName != "" {
		agent, err := startAgentTmux(dir, command, args, prompt, stationName, repoDir)
		if err == nil {
			return agent, nil
		}
		// tmux setup failed — fall back to direct execution
		fmt.Fprintf(os.Stderr, "assembly-line: tmux setup failed, falling back to direct: %v\n", err)
	}
	return startAgentDirect(dir, command, args, prompt)
}

// startAgentDirect launches an agent as a direct subprocess (original behavior).
func startAgentDirect(dir, command string, args []string, prompt string) (*agentProcess, error) {
	fullPrompt := preamble + "\n\n" + prompt
	fullArgs := make([]string, len(args))
	copy(fullArgs, args)
	fullArgs = append(fullArgs, fullPrompt)

	cmd := exec.Command(command, fullArgs...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Build a clean environment for the agent:
	// - Remove CLAUDECODE so Claude Code can launch as a fresh session
	// - Set LINE_RUNNING=1 to prevent retriggering
	env := cleanEnv(os.Environ(), "CLAUDECODE")
	cmd.Env = append(env, "LINE_RUNNING=1")

	// Set process group so we can kill the whole group
	setProcGroup(cmd)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting agent %q: %w", command, err)
	}

	return &agentProcess{cmd: cmd}, nil
}

// isClaudeCommand returns true if the command basename is "claude".
func isClaudeCommand(command string) bool {
	base := filepath.Base(command)
	return base == "claude" || strings.HasPrefix(base, "claude-")
}

// startAgentTmux launches an agent inside a tmux session for observability.
func startAgentTmux(dir, command string, args []string, prompt, stationName, repoDir string) (*agentProcess, error) {
	sessionName := tmux.SessionName(repoDir, stationName)
	claudeMode := isClaudeCommand(command)

	// Kill any stale session with that name
	_ = tmux.KillSession(sessionName)

	// Build the full command line for the shell.
	fullPrompt := preamble + "\n\n" + prompt
	var fullArgs []string
	for _, a := range args {
		// Drop -p/--print for Claude Code: the tmux PTY provides interactive
		// mode with streaming output. In -p mode Claude Code batches all
		// output until exit, defeating the purpose of tmux observability.
		if claudeMode && (a == "-p" || a == "--print") {
			continue
		}
		fullArgs = append(fullArgs, a)
	}
	fullArgs = append(fullArgs, fullPrompt)

	shellCmd := shellescape(command)
	for _, a := range fullArgs {
		shellCmd += " " + shellescape(a)
	}

	// Prepend environment setup to the shell command
	envPrefix := "export LINE_RUNNING=1; unset CLAUDECODE; "
	shellCmd = envPrefix + shellCmd

	// Create the tmux session (remain-on-exit is set atomically by NewSession)
	if err := tmux.NewSession(sessionName, dir, shellCmd); err != nil {
		return nil, fmt.Errorf("creating tmux session: %w", err)
	}

	// For Claude Code without -p: accept the workspace trust dialog.
	// Claude shows an interactive trust prompt on first visit to a directory.
	// Sending Enter selects the default "Yes, I trust this folder" option.
	if claudeMode {
		time.Sleep(2 * time.Second)
		_ = tmux.SendKeys(sessionName, "")
	}

	// Set up pipe-pane to stream output to a log file
	logPath := state.StationLogPath(repoDir, stationName)
	if err := tmux.PipePane(sessionName, "cat >> "+shellescape(logPath)); err != nil {
		_ = tmux.KillSession(sessionName)
		return nil, fmt.Errorf("setting up pipe-pane: %w", err)
	}

	// Get pane PID for state tracking
	panePID, err := tmux.PanePID(sessionName)
	if err != nil {
		_ = tmux.KillSession(sessionName)
		return nil, fmt.Errorf("getting pane PID: %w", err)
	}

	return &agentProcess{
		tmuxSession:  sessionName,
		tmuxPanePID:  panePID,
		logPath:      logPath,
		stationName:  stationName,
		repoDir:      repoDir,
		isClaudeCode: claudeMode,
	}, nil
}

// wait waits for the agent to finish.
func (a *agentProcess) wait() error {
	if a.tmuxSession != "" {
		return a.waitTmux()
	}
	return a.cmd.Wait()
}

// waitTmux polls the tmux pane until the process exits.
// For Claude Code (interactive mode, no -p): detects the idle "❯" prompt
// and sends /exit to cleanly shut down. For other commands: waits for
// natural pane death.
func (a *agentProcess) waitTmux() error {
	exitSent := false
	// Grace period: don't check for idle prompt until Claude Code has had
	// time to start processing (avoids matching the initial prompt display).
	idleCheckAfter := time.Now().Add(15 * time.Second)
	for {
		dead, exitCode, err := tmux.PaneStatus(a.tmuxSession)
		if err != nil {
			// Session may have been killed externally
			return fmt.Errorf("checking tmux pane status: %w", err)
		}
		if dead {
			_ = tmux.KillSession(a.tmuxSession)
			if exitCode != 0 {
				return fmt.Errorf("exit status %d", exitCode)
			}
			return nil
		}

		// For Claude Code: detect the idle ❯ prompt and send /exit.
		if a.isClaudeCode && !exitSent && time.Now().After(idleCheckAfter) {
			if captured, err := tmux.CapturePaneLines(a.tmuxSession, 10); err == nil {
				if isIdlePrompt(captured) {
					// Claude Code's TUI intercepts "/" and shows an autocomplete
					// picker. The first Enter selects "/exit" from the menu;
					// a second Enter after a brief pause executes the command.
					_ = tmux.SendKeys(a.tmuxSession, "/exit")
					time.Sleep(500 * time.Millisecond)
					_ = tmux.SendKeys(a.tmuxSession, "")
					exitSent = true
				}
			}
		}

		time.Sleep(500 * time.Millisecond)
	}
}

// isIdlePrompt checks if capture-pane output indicates Claude Code is idle
// at its input prompt (❯) and not actively processing.
//
// Claude Code's TUI has a status bar at the bottom of the pane (with info like
// permissions mode, active tools, etc.), so the bare ❯ prompt line is NOT the
// last non-empty line. We scan all captured lines for a bare ❯.
func isIdlePrompt(captured string) bool {
	for _, line := range strings.Split(captured, "\n") {
		line = strings.TrimSpace(line)
		// Claude Code shows "❯" alone when idle at the input prompt.
		// When displaying the initial prompt, it shows "❯ <prompt text>".
		// We only match the bare prompt with no text following it.
		if line == "❯" {
			return true
		}
	}
	return false
}

// pid returns the process ID of the agent.
func (a *agentProcess) pid() int {
	if a.tmuxSession != "" {
		return a.tmuxPanePID
	}
	if a.cmd != nil && a.cmd.Process != nil {
		return a.cmd.Process.Pid
	}
	return 0
}

// session returns the tmux session name, or "" if not using tmux.
func (a *agentProcess) session() string {
	return a.tmuxSession
}

// shellescape wraps a string in single quotes for safe shell interpolation.
// Embedded single quotes are escaped as '\'' (end quote, escaped quote, start quote).
func shellescape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// cleanEnv returns a copy of environ with the named variables removed.
func cleanEnv(environ []string, keys ...string) []string {
	result := make([]string, 0, len(environ))
	for _, e := range environ {
		skip := false
		for _, key := range keys {
			if strings.HasPrefix(e, key+"=") {
				skip = true
				break
			}
		}
		if !skip {
			result = append(result, e)
		}
	}
	return result
}
