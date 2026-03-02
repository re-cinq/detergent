package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/re-cinq/assembly-line/internal/state"
)

// tempRepo creates a fresh git repo in a temp directory and returns its path.
// The directory is cleaned up after the test.
func tempRepo() string {
	dir, err := os.MkdirTemp("", "line-test-*")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { os.RemoveAll(dir) })

	git(dir, "init")
	git(dir, "config", "user.email", "test@test.com")
	git(dir, "config", "user.name", "Test")
	// Create an initial commit so HEAD exists
	writeFile(dir, "README.md", "# test\n")
	git(dir, "add", ".")
	git(dir, "commit", "-m", "initial commit")

	return dir
}

// gitMay runs a git command that may fail and returns stdout+err.
func gitMay(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// git runs a git command in the given directory and returns stdout.
func git(dir string, args ...string) string {
	out, err := gitMay(dir, args...)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "git %s failed: %s", strings.Join(args, " "), out)
	return out
}

// line runs the line binary in the given directory and returns stdout.
func line(dir string, args ...string) (string, error) {
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// lineOK runs the line binary and expects success.
func lineOK(dir string, args ...string) string {
	out, err := line(dir, args...)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "line %s failed: %s", strings.Join(args, " "), out)
	return out
}

// writeFile creates a file with the given content, creating parent dirs as needed.
func writeFile(dir, name, content string) {
	p := filepath.Join(dir, name)
	err := os.MkdirAll(filepath.Dir(p), 0o755)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	err = os.WriteFile(p, []byte(content), 0o644)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

// readFile reads a file and returns its content.
func readFile(dir, name string) string {
	data, err := os.ReadFile(filepath.Join(dir, name))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return string(data)
}

// writeConfig writes a line.yaml config to the given directory.
func writeConfig(dir string, content string) {
	writeFile(dir, "line.yaml", content)
}

// writeDefaultConfig writes a minimal config for testing.
func writeDefaultConfig(dir string) {
	writeConfig(dir, `agent:
  command: echo
  args: ["hello"]

settings:
  watches: master

gates:
  - name: lint
    run: "true"

stations:
  - name: review
    prompt: "Review code"
`)
}

// writeMockAgentScript writes an executable shell script with the given content.
func writeMockAgentScript(dir, filename, content string) string {
	script := filepath.Join(dir, filename)
	err := os.WriteFile(script, []byte(content), 0o755)
	ExpectWithOffset(2, err).NotTo(HaveOccurred())
	return script
}

// writeMockAgent writes a mock agent script that modifies files predictably.
func writeMockAgent(dir string) string {
	return writeMockAgentScript(dir, "mock-agent.sh", `#!/bin/bash
# Mock agent: reads the prompt from the last argument, creates a file
PROMPT="${@: -1}"
echo "mock-agent ran with prompt: $PROMPT"
echo "agent was here: $PROMPT" >> agent-output.txt
`)
}

// writeFailingMockAgent writes a mock agent that exits with a non-zero code.
func writeFailingMockAgent(dir string) string {
	return writeMockAgentScript(dir, "failing-agent.sh", `#!/bin/bash
# Failing mock agent: exits with non-zero status
PROMPT="${@: -1}"
echo "failing-agent ran with prompt: $PROMPT"
exit 1
`)
}

// writeSlowMockAgent writes a mock agent that sleeps for testing RUN-11.
func writeSlowMockAgent(dir string) string {
	return writeMockAgentScript(dir, "slow-agent.sh", `#!/bin/bash
# Slow mock agent for testing process termination
PROMPT="${@: -1}"
echo "slow-agent started with prompt: $PROMPT"
echo "agent was here: $PROMPT" >> agent-output.txt
sleep 30
`)
}

// fileExists checks if a file exists in the given directory.
func fileExists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

// currentBranch returns the current branch name.
func currentBranch(dir string) string {
	return git(dir, "rev-parse", "--abbrev-ref", "HEAD")
}

// shortRef returns the short ref of HEAD.
func shortRef(dir string) string {
	return git(dir, "rev-parse", "--short", "HEAD")
}

// installHooksForTest installs hooks (via init) and patches
// them to use the full test binary path. The post-commit hook runs in the
// foreground so git commit blocks until line run completes.
func installHooksForTest(dir string) {
	lineOK(dir, "init")
	patchHook(dir, "post-commit", "line run &", binaryPath+" run")
	patchHook(dir, "pre-commit", "line gate", binaryPath+" gate")
}

// installHooksForTestBg is like installHooksForTest but keeps the post-commit
// hook backgrounded (&). Output is redirected to .line/run.log so that
// git commit's CombinedOutput pipe closes promptly (the background process
// would otherwise hold the pipe open, causing the test to block).
func installHooksForTestBg(dir string) {
	lineOK(dir, "init")
	// Ensure .line/ dir exists for the log file
	logDir := filepath.Join(dir, ".line")
	err := os.MkdirAll(logDir, 0o755)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	logFile := filepath.Join(logDir, "run.log")
	patchHook(dir, "post-commit", "line run &", binaryPath+" run >"+logFile+" 2>&1 &")
	patchHook(dir, "pre-commit", "line gate", binaryPath+" gate")
}

// patchHook replaces old with new in the named hook file.
func patchHook(dir, hook, old, repl string) {
	path := filepath.Join(dir, ".git", "hooks", hook)
	data, err := os.ReadFile(path)
	ExpectWithOffset(2, err).NotTo(HaveOccurred())
	patched := strings.Replace(string(data), old, repl, 1)
	err = os.WriteFile(path, []byte(patched), 0o755)
	ExpectWithOffset(2, err).NotTo(HaveOccurred())
}

// gitCommit stages all changes and commits, triggering any installed hooks.
// Returns the combined output including hook stderr.
func gitCommit(dir, message string) string {
	git(dir, "add", ".")
	return git(dir, "commit", "-m", message)
}

// killBackground kills background line processes for the given directory.
// It kills the line runner (run.pid) and the named station (stations/<name>.pid).
// If the station was running inside tmux, the tmux session is also killed.
func killBackground(dir, stationName string) {
	// Kill tmux session if present
	if sessionName := state.ReadStationTmux(dir, stationName); sessionName != "" {
		state.KillTmuxSession(sessionName)
		_ = state.RemoveStationTmux(dir, stationName)
	}
	if pid, err := state.ReadPID(dir); err == nil && pid > 0 {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	if pid, _, err := state.ReadStationPID(dir, stationName); err == nil && pid > 0 {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}
