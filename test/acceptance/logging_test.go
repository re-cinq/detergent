package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("agent output logging", func() {
	var tmpDir string
	var repoDir string
	var configPath string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "detergent-logging-*")
		Expect(err).NotTo(HaveOccurred())

		repoDir = filepath.Join(tmpDir, "repo")
		runGit(tmpDir, "init", repoDir)
		runGit(repoDir, "checkout", "-b", "main")
		writeFile(filepath.Join(repoDir, "hello.txt"), "hello\n")
		runGit(repoDir, "add", "hello.txt")
		runGit(repoDir, "commit", "-m", "initial commit")
	})

	AfterEach(func() {
		cleanupTestRepo(repoDir, tmpDir)
		// Clean up log files
		os.Remove(filepath.Join(os.TempDir(), "detergent-security.log"))
		os.Remove(filepath.Join(os.TempDir(), "detergent-style.log"))
	})

	Describe("run --once", func() {
		It("writes agent output to log file, not terminal", func() {
			configPath = filepath.Join(repoDir, "detergent.yaml")
			writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "echo 'AGENT_OUTPUT_MARKER' && echo 'reviewed' > review.txt"]

settings:
  branch_prefix: "detergent/"

concerns:
  - name: security
    watches: main
    prompt: "Review for security issues"
`)

			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

			// Agent output should NOT appear on terminal
			Expect(string(output)).NotTo(ContainSubstring("AGENT_OUTPUT_MARKER"))

			// Agent output SHOULD appear in log file
			logPath := filepath.Join(os.TempDir(), "detergent-security.log")
			logContent, err := os.ReadFile(logPath)
			Expect(err).NotTo(HaveOccurred(), "log file should exist")
			Expect(string(logContent)).To(ContainSubstring("AGENT_OUTPUT_MARKER"))
		})

		It("creates separate log files per concern", func() {
			configPath = filepath.Join(repoDir, "detergent.yaml")
			writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "echo 'OUTPUT_FROM_'$DETERGENT_CONCERN && touch reviewed.txt"]

settings:
  branch_prefix: "detergent/"

concerns:
  - name: security
    watches: main
    prompt: "Security review"
  - name: style
    watches: main
    prompt: "Style review"
`)

			// Set env var so agent can report which concern it's running for
			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			cmd.Env = append(os.Environ(), "DETERGENT_CONCERN=test")
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

			// Both log files should exist
			securityLog := filepath.Join(os.TempDir(), "detergent-security.log")
			styleLog := filepath.Join(os.TempDir(), "detergent-style.log")

			_, err = os.Stat(securityLog)
			Expect(err).NotTo(HaveOccurred(), "security log file should exist")

			_, err = os.Stat(styleLog)
			Expect(err).NotTo(HaveOccurred(), "style log file should exist")
		})

		It("includes commit hash header in log file", func() {
			configPath = filepath.Join(repoDir, "detergent.yaml")
			writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "echo 'agent ran' > review.txt"]

settings:
  branch_prefix: "detergent/"

concerns:
  - name: security
    watches: main
    prompt: "Review for security"
`)

			// Get the commit hash we'll be processing
			commitHash := strings.TrimSpace(runGitOutput(repoDir, "rev-parse", "HEAD"))

			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

			logPath := filepath.Join(os.TempDir(), "detergent-security.log")
			logContent, err := os.ReadFile(logPath)
			Expect(err).NotTo(HaveOccurred())

			// Should contain the header with commit hash
			Expect(string(logContent)).To(ContainSubstring("--- Processing " + commitHash))
		})
	})

	Describe("PTY support", func() {
		It("provides a TTY to the agent stdout", func() {
			configPath = filepath.Join(repoDir, "detergent.yaml")
			writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "test -t 1 && echo TTY_YES || echo TTY_NO"]

settings:
  branch_prefix: "detergent/"

concerns:
  - name: security
    watches: main
    prompt: "Review"
`)

			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

			logPath := filepath.Join(os.TempDir(), "detergent-security.log")
			logContent, err := os.ReadFile(logPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(logContent)).To(ContainSubstring("TTY_YES"),
				"agent should see stdout as a TTY (PTY allocated)")
		})

		It("streams agent output to log file in real-time", func() {
			if _, err := exec.LookPath("python3"); err != nil {
				Skip("python3 not found")
			}

			configPath = filepath.Join(repoDir, "detergent.yaml")
			// Python uses full buffering on pipes, line buffering on TTYs.
			// Without the PTY, STREAMING_MARKER would stay in Python's
			// buffer and not appear in the log until the process exits
			// (5 seconds later). With the PTY, it flushes immediately.
			writeFile(configPath, `
agent:
  command: "python3"
  args: ["-c", "import time; print('STREAMING_MARKER'); time.sleep(5)"]

settings:
  branch_prefix: "detergent/"

concerns:
  - name: security
    watches: main
    prompt: "Review"
`)

			logPath := filepath.Join(os.TempDir(), "detergent-security.log")
			os.Remove(logPath)

			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			err := cmd.Start()
			Expect(err).NotTo(HaveOccurred())
			defer cmd.Process.Kill()

			// Poll for the marker within 3s. The agent sleeps 5s total,
			// so if it appears before the deadline the output was streamed
			// in real-time (not buffered until agent exit).
			Eventually(func() string {
				data, _ := os.ReadFile(logPath)
				return string(data)
			}, 3*time.Second, 200*time.Millisecond).Should(
				ContainSubstring("STREAMING_MARKER"),
				"agent output should appear in log before process exits (real-time streaming)")

			// Verify detergent is still running (agent hasn't exited yet)
			Expect(cmd.Process.Signal(syscall.Signal(0))).To(Succeed(),
				"detergent should still be running — output was streamed, not buffered until exit")

			cmd.Wait()
		})
	})

	Describe("daemon mode", func() {
		It("shows daemon messages on terminal but not agent output", func() {
			configPath = filepath.Join(repoDir, "detergent.yaml")
			writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "echo 'AGENT_SECRET_OUTPUT' && touch reviewed.txt"]

settings:
  poll_interval: 1s
  branch_prefix: "detergent/"

concerns:
  - name: security
    watches: main
    prompt: "Review"
`)

			// Remove stale log file from prior tests (shared /tmp path)
			logPath := filepath.Join(os.TempDir(), "detergent-security.log")
			os.Remove(logPath)

			cmd := exec.Command(binaryPath, "run", "--path", configPath)
			cmd.Dir = repoDir
			var outputBuf strings.Builder
			cmd.Stdout = &outputBuf
			cmd.Stderr = &outputBuf

			err := cmd.Start()
			Expect(err).NotTo(HaveOccurred())

			// Wait for processing
			time.Sleep(2 * time.Second)

			// Send SIGINT
			cmd.Process.Signal(syscall.SIGINT)
			cmd.Wait()

			terminalOutput := outputBuf.String()

			// Daemon messages should appear on terminal
			Expect(terminalOutput).To(ContainSubstring("daemon started"))
			Expect(terminalOutput).To(ContainSubstring("Agent logs:"))

			// Agent output should NOT appear on terminal
			Expect(terminalOutput).NotTo(ContainSubstring("AGENT_SECRET_OUTPUT"))

			// Agent output SHOULD appear in log file
			logContent, err := os.ReadFile(logPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(logContent)).To(ContainSubstring("AGENT_SECRET_OUTPUT"))
		})
	})
})
