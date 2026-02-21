package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("line status", func() {
	var tmpDir string
	var repoDir string
	var configPath string

	BeforeEach(func() {
		tmpDir, repoDir = setupTestRepo("line-status-*")

		configPath = filepath.Join(repoDir, "line.yaml")
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

	Context("runner status", func() {
		It("shows runner as not running when no runner is alive", func() {
			cmd := exec.Command(binaryPath, "status", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("inactive"))
		})
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

		It("shows the source branch with commit shortref", func() {
			head := runGitOutput(repoDir, "rev-parse", "main")

			cmd := exec.Command(binaryPath, "status", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			out := string(output)
			// Source branch name and shortref should appear before concerns
			Expect(out).To(ContainSubstring("main"))
			Expect(out).To(ContainSubstring(head[:8]))
		})

		It("shows dirty indicator when workspace has uncommitted changes", func() {
			// Create an untracked file to make workspace dirty
			writeFile(filepath.Join(repoDir, "dirty.txt"), "uncommitted change")

			cmd := exec.Command(binaryPath, "status", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("dirty"))
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
			statusDir := filepath.Join(repoDir, ".line", "status")
			Expect(os.MkdirAll(statusDir, 0755)).To(Succeed())
			writeFile(filepath.Join(statusDir, "security.json"),
				`{"state":"agent_running","started_at":"2025-01-01T00:00:00Z","head_at_start":"abc123","pid":99999}`)

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
			failConfigPath := filepath.Join(repoDir, "line.yaml")
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
