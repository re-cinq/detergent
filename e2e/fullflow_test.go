package e2e_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/re-cinq/assembly-line/internal/tmux"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("full auto-rebase flow", Label("claude"), func() {
	var dir string

	BeforeEach(func() {
		if _, err := exec.LookPath("claude"); err != nil {
			Skip("claude CLI not found, skipping integration test")
		}
		dir = tempRepo()
	})

	It("auto-rebases master when Claude triggers a hook [FULLFLOW-1]", func() {
		// --- Setup: config with mock agent station + auto_rebase ---
		agentScript := writeMockAgentScript(dir, "station-agent.sh", `#!/bin/bash
echo "station-was-here" > station-was-here.txt
`)

		writeConfig(dir, `agent:
  command: echo
  args: ["hello"]

settings:
  watches: master
  auto_rebase: true

stations:
  - name: review
    command: `+agentScript+`
    args: ["-p"]
    prompt: "Review code"
`)

		// Init installs git hooks + .claude/settings.json
		lineOK(dir, "init")

		// Only patch the post-commit hook: redirect output to a log file
		// (so Claude's Bash tool doesn't block) and strip LINE_RUNNING
		// (which may leak from the tmux server environment).
		logDir := filepath.Join(dir, ".line")
		Expect(os.MkdirAll(logDir, 0o755)).To(Succeed())
		logFile := filepath.Join(logDir, "run.log")
		patchHook(dir, "post-commit", "line run &",
			"env -u LINE_RUNNING line run >"+logFile+" 2>&1 &")

		// Commit config files so the station worktree has them.
		// Without this, the pre-commit hook (line gate) fails in the
		// worktree because line.yaml doesn't exist there.
		git(dir, "add", "line.yaml", ".claude", ".gitignore")
		git(dir, "commit", "-m", "add assembly-line config [skip line]")

		// Record master HEAD before prompt.
		headBeforePrompt := git(dir, "rev-parse", "HEAD")

		// --- Start Claude Code in tmux ---
		// Put the test binary on PATH so all bare "line" commands work
		// (including after the runner re-writes settings.json with bare
		// "line auto-rebase-hook"). Also unset CLAUDECODE (prevents nested
		// session error) and LINE_RUNNING (prevents run from skipping).
		sessionName := "line-fullflow-test"
		DeferCleanup(func() { tmux.KillSession(sessionName) })

		binDir := filepath.Dir(binaryPath)
		shellCmd := fmt.Sprintf(
			"env -u CLAUDECODE -u LINE_RUNNING PATH=%s:$PATH "+
				"claude --dangerously-skip-permissions --model haiku",
			binDir)
		err := tmux.NewSession(sessionName, dir, shellCmd)
		Expect(err).NotTo(HaveOccurred())

		// Give Claude a moment to start.
		time.Sleep(5 * time.Second)
		_ = tmux.SendKeys(sessionName, "")

		// --- Prompt 1: ask Claude to create a file and commit ---
		// This fires: post-commit hook → line run → station creates
		// station-was-here.txt → Claude's Stop hook → auto-rebase-hook
		// → master advances with station changes.
		//
		// Send text with -l (literal) flag to avoid key name interpretation,
		// then send Enter separately to ensure submission.
		time.Sleep(2 * time.Second)
		prompt1 := "Create a file called trigger.txt containing hello then git add and git commit -m 'add trigger'"
		Expect(exec.Command("tmux", "send-keys", "-t", sessionName, "-l", prompt1).Run()).To(Succeed())
		time.Sleep(500 * time.Millisecond)
		Expect(exec.Command("tmux", "send-keys", "-t", sessionName, "Enter").Run()).To(Succeed())

		// --- Wait for station-was-here.txt on master ---
		// The auto-rebase hook fires on Claude's PostToolUse or Stop,
		// rebasing master onto the terminal station. Once station-was-here.txt
		// appears on master, the full flow is proven.
		Eventually(func() bool {
			return fileExists(dir, "station-was-here.txt")
		}, 90*time.Second, 2*time.Second).Should(BeTrue(),
			"timed out waiting for station-was-here.txt to appear on master via auto-rebase")

		// --- Verify ---
		Expect(readFile(dir, "station-was-here.txt")).To(ContainSubstring("station-was-here"))
		Expect(currentBranch(dir)).To(Equal("master"))

		// Master HEAD must have advanced past the pre-prompt snapshot.
		headAfter := git(dir, "rev-parse", "HEAD")
		Expect(headAfter).NotTo(Equal(headBeforePrompt),
			"master HEAD should have advanced after auto-rebase")
	})
})
