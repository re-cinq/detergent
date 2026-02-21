package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// filterOutEnv removes variables matching the given prefix from the environment.
func filterOutEnv(env []string, prefix string) []string {
	result := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			result = append(result, e)
		}
	}
	return result
}

var _ = Describe("line trigger", func() {
	var tmpDir string
	var repoDir string
	var configPath string

	BeforeEach(func() {
		tmpDir, repoDir = setupTestRepo("line-trigger-*")

		configPath = filepath.Join(repoDir, "line.yaml")
		writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "echo ok"]

settings:

stations:
  - name: security
    watches: main
    prompt: "check"
`)
	})

	AfterEach(func() {
		cleanupTestRepo(repoDir, tmpDir)
	})

	It("writes trigger file with HEAD hash", func() {
		cmd := exec.Command(binaryPath, "trigger", "--path", configPath)
		cmd.Dir = repoDir
		// Clear LINE_AGENT so the trigger command doesn't skip
		cmd.Env = filterOutEnv(os.Environ(), "LINE_AGENT=")
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "trigger failed: %s", string(output))

		triggerPath := filepath.Join(repoDir, ".line", "trigger")
		data, err := os.ReadFile(triggerPath)
		Expect(err).NotTo(HaveOccurred())

		head := strings.TrimSpace(runGitOutput(repoDir, "rev-parse", "HEAD"))
		Expect(strings.TrimSpace(string(data))).To(Equal(head))
	})

	It("skips spawn when runner already alive", func() {
		// Write a fake PID file with our own PID (which is alive)
		lineDir := filepath.Join(repoDir, ".line")
		Expect(os.MkdirAll(lineDir, 0o755)).To(Succeed())
		writeFile(filepath.Join(lineDir, "runner.pid"), fmt.Sprintf("%d\n", os.Getpid()))

		cmd := exec.Command(binaryPath, "trigger", "--path", configPath)
		cmd.Dir = repoDir
		// Clear LINE_AGENT so the trigger command doesn't skip
		cmd.Env = filterOutEnv(os.Environ(), "LINE_AGENT=")
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "trigger failed: %s", string(output))

		// The PID file should still reference our test process, not a new runner
		data, err := os.ReadFile(filepath.Join(lineDir, "runner.pid"))
		Expect(err).NotTo(HaveOccurred())
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		Expect(err).NotTo(HaveOccurred())
		Expect(pid).To(Equal(os.Getpid()))
	})
})
