package acceptance_test

import (
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("parallel concern execution", func() {
	var tmpDir string
	var repoDir string
	var configPath string

	BeforeEach(func() {
		tmpDir, repoDir = setupTestRepo("line-parallel-*")
	})

	AfterEach(func() {
		cleanupTestRepo(repoDir, tmpDir)
	})

	Context("with two independent concerns that both watch main", func() {
		BeforeEach(func() {
			// Each agent sleeps 1 second then writes a file.
			// If parallel: total ~1s. If serial: total ~2s.
			configPath = filepath.Join(repoDir, "line.yaml")
			writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "sleep 1 && date +%s%N > agent-output.txt"]

concerns:
  - name: alpha
    watches: main
    prompt: "Check alpha"
  - name: beta
    watches: main
    prompt: "Check beta"
`)
		})

		It("processes both concerns", func() {
			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

			branches := runGitOutput(repoDir, "branch")
			Expect(branches).To(ContainSubstring("line/alpha"))
			Expect(branches).To(ContainSubstring("line/beta"))
		})

		It("runs independent concerns concurrently (faster than serial)", func() {
			start := time.Now()
			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			output, err := cmd.CombinedOutput()
			elapsed := time.Since(start)
			Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

			// If serial, would take ~2s (1s per agent). Parallel should be ~1s.
			// Allow up to 1.8s for parallel (with overhead) - if it takes more, it's serial.
			Expect(elapsed).To(BeNumerically("<", 1800*time.Millisecond),
				"expected parallel execution to complete in <1.8s, took %s", elapsed)
		})
	})

	Context("with dependent concerns (A -> B)", func() {
		BeforeEach(func() {
			configPath = filepath.Join(repoDir, "line.yaml")
			writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "echo done > agent-output.txt"]

concerns:
  - name: upstream
    watches: main
    prompt: "First pass"
  - name: downstream
    watches: upstream
    prompt: "Second pass"
`)
		})

		It("processes dependent concerns sequentially", func() {
			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

			// Both should complete, with downstream seeing upstream's output
			branches := runGitOutput(repoDir, "branch")
			Expect(branches).To(ContainSubstring("line/upstream"))
			Expect(branches).To(ContainSubstring("line/downstream"))

			// Downstream's Triggered-By should reference upstream's branch
			msg := runGitOutput(repoDir, "log", "-1", "--format=%B", "line/downstream")
			Expect(msg).To(ContainSubstring("Triggered-By:"))
		})
	})
})
