package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("detergent status", func() {
	var tmpDir string
	var repoDir string
	var configPath string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "detergent-status-*")
		Expect(err).NotTo(HaveOccurred())

		repoDir = filepath.Join(tmpDir, "repo")
		runGit(tmpDir, "init", repoDir)
		runGit(repoDir, "checkout", "-b", "main")
		writeFile(filepath.Join(repoDir, "hello.txt"), "hello\n")
		runGit(repoDir, "add", "hello.txt")
		runGit(repoDir, "commit", "-m", "initial commit")

		configPath = filepath.Join(repoDir, "detergent.yaml")
		writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "echo 'reviewed' > agent-review.txt"]

concerns:
  - name: security
    watches: main
    prompt: "Review for security issues"
`)
	})

	AfterEach(func() {
		cleanupTestRepo(repoDir, tmpDir)
	})

	Context("before any run", func() {
		It("shows concerns as pending", func() {
			cmd := exec.Command(binaryPath, "status", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			out := string(output)
			Expect(out).To(ContainSubstring("security"))
			Expect(out).To(ContainSubstring("pending"))
		})
	})

	Context("after a successful run", func() {
		BeforeEach(func() {
			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			out, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "run failed: %s", string(out))
		})

		It("shows concerns as caught up", func() {
			cmd := exec.Command(binaryPath, "status", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			out := string(output)
			Expect(out).To(ContainSubstring("security"))
			Expect(out).To(ContainSubstring("caught up"))
		})

		It("shows the last-processed commit hash", func() {
			// Get the main branch HEAD
			head := runGitOutput(repoDir, "rev-parse", "main")

			cmd := exec.Command(binaryPath, "status", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			// Should contain first 8 chars of the hash
			Expect(string(output)).To(ContainSubstring(head[:8]))
		})
	})

	Context("with a stale active state (dead PID)", func() {
		It("shows stale status when process is dead", func() {
			// Write a fake status file with agent_running state and a dead PID
			statusDir := filepath.Join(repoDir, ".detergent", "status")
			Expect(os.MkdirAll(statusDir, 0755)).To(Succeed())
			writeFile(filepath.Join(statusDir, "security.json"),
				`{"state":"agent_running","started_at":"2025-01-01T00:00:00Z","head_at_start":"abc123","last_seen":"","pid":99999}`)

			cmd := exec.Command(binaryPath, "status", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			out := string(output)
			Expect(out).To(ContainSubstring("stale"))
			Expect(out).To(ContainSubstring("99999"))
		})
	})

	Context("with failed concern", func() {
		It("shows failed status with error message", func() {
			failConfigPath := filepath.Join(repoDir, "detergent.yaml")
			writeFile(failConfigPath, `
agent:
  command: "sh"
  args: ["-c", "exit 1"]

concerns:
  - name: security
    watches: main
    prompt: "This will fail"
`)
			cmd := exec.Command(binaryPath, "run", "--once", "--path", failConfigPath)
			cmd.CombinedOutput() // ignore exit code

			cmd2 := exec.Command(binaryPath, "status", "--path", failConfigPath)
			output, err := cmd2.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("failed"))
		})
	})
})
