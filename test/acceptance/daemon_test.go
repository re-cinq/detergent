package acceptance_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("detergent run (daemon mode)", func() {
	var tmpDir string
	var repoDir string
	var configPath string

	BeforeEach(func() {
		tmpDir, repoDir = setupTestRepo("detergent-daemon-*")

		// Use a very short poll interval for testing
		configPath = filepath.Join(repoDir, "detergent.yaml")
		writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "echo 'daemon-reviewed' > daemon-review.txt"]

settings:
  poll_interval: 1s

concerns:
  - name: security
    watches: main
    prompt: "Review for security issues"
`)
	})

	AfterEach(func() {
		cleanupTestRepo(repoDir, tmpDir)
	})

	It("processes commits and exits cleanly on SIGINT", func() {
		// Start daemon in background
		cmd := exec.Command(binaryPath, "run", "--path", configPath)
		cmd.Dir = repoDir
		var outputBuf strings.Builder
		cmd.Stdout = &outputBuf
		cmd.Stderr = &outputBuf

		err := cmd.Start()
		Expect(err).NotTo(HaveOccurred())

		// Wait for initial processing
		time.Sleep(2 * time.Second)

		// Verify the initial commit was processed
		branchOut := runGitOutput(repoDir, "branch", "--list", "detergent/security")
		Expect(branchOut).To(ContainSubstring("detergent/security"))

		// Make a new commit on main
		writeFile(filepath.Join(repoDir, "new-file.txt"), "new content\n")
		runGit(repoDir, "add", "new-file.txt")
		runGit(repoDir, "commit", "-m", "second commit")

		// Wait for the daemon to pick it up (next poll cycle)
		time.Sleep(3 * time.Second)

		// Send SIGINT for clean shutdown
		cmd.Process.Signal(syscall.SIGINT)
		err = cmd.Wait()

		// The process should exit cleanly (exit 0)
		Expect(err).NotTo(HaveOccurred(), "daemon output: %s", outputBuf.String())

		// Verify the output includes daemon startup message
		Expect(outputBuf.String()).To(ContainSubstring("daemon started"))

		// Verify the second commit was processed
		commitCount := runGitOutput(repoDir, "rev-list", "--count", "detergent/security")
		// Initial commit creates branch (1 ancestor) + agent commit + second trigger's agent commit
		// At minimum should have more than 1 commit on the output branch
		count := strings.TrimSpace(commitCount)
		Expect(count).NotTo(Equal("1"), "expected daemon to process additional commits")
	})
})
