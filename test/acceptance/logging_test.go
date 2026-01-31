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
		exec.Command("git", "-C", repoDir, "worktree", "prune").Run()
		os.RemoveAll(tmpDir)
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

			cmd := exec.Command(binaryPath, "run", "--once", configPath)
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
			cmd := exec.Command(binaryPath, "run", "--once", configPath)
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

			cmd := exec.Command(binaryPath, "run", "--once", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

			logPath := filepath.Join(os.TempDir(), "detergent-security.log")
			logContent, err := os.ReadFile(logPath)
			Expect(err).NotTo(HaveOccurred())

			// Should contain the header with commit hash
			Expect(string(logContent)).To(ContainSubstring("--- Processing " + commitHash))
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

			cmd := exec.Command(binaryPath, "run", configPath)
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
			logPath := filepath.Join(os.TempDir(), "detergent-security.log")
			logContent, err := os.ReadFile(logPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(logContent)).To(ContainSubstring("AGENT_SECRET_OUTPUT"))
		})
	})
})
