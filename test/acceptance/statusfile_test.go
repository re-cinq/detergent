package acceptance_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type concernStatus struct {
	State       string `json:"state"`
	LastResult  string `json:"last_result,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	Error       string `json:"error,omitempty"`
	LastSeen    string `json:"last_seen,omitempty"`
	HeadAtStart string `json:"head_at_start,omitempty"`
	PID         int    `json:"pid"`
}

var _ = Describe("status files", func() {
	var tmpDir string
	var repoDir string
	var configPath string

	BeforeEach(func() {
		tmpDir, repoDir = setupTestRepo("line-statusfile-*")
	})

	AfterEach(func() {
		cleanupTestRepo(repoDir, tmpDir)
	})

	readStatus := func(concernName string) *concernStatus {
		path := filepath.Join(repoDir, ".line", "status", concernName+".json")
		data, err := os.ReadFile(path)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "status file not found: %s", path)
		var s concernStatus
		ExpectWithOffset(1, json.Unmarshal(data, &s)).To(Succeed())
		return &s
	}

	Context("after a successful run with changes", func() {
		BeforeEach(func() {
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
			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			out, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "run failed: %s", string(out))
		})

		It("writes a status file for the concern", func() {
			s := readStatus("security")
			Expect(s.State).To(Equal("idle"))
			Expect(s.LastResult).To(Equal("modified"))
			Expect(s.PID).To(BeNumerically(">", 0))
			Expect(s.StartedAt).NotTo(BeEmpty())
			Expect(s.CompletedAt).NotTo(BeEmpty())
			Expect(s.LastSeen).NotTo(BeEmpty())
		})
	})

	Context("after a successful run with no changes (noop)", func() {
		BeforeEach(func() {
			configPath = filepath.Join(repoDir, "line.yaml")
			writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "echo noop"]

concerns:
  - name: style
    watches: main
    prompt: "Check style"
`)
			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			out, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "run failed: %s", string(out))
		})

		It("writes status with noop result", func() {
			s := readStatus("style")
			Expect(s.State).To(Equal("idle"))
			Expect(s.LastResult).To(Equal("noop"))
		})
	})

	Context("when concern is already caught up", func() {
		BeforeEach(func() {
			configPath = filepath.Join(repoDir, "line.yaml")
			writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "echo noop"]

concerns:
  - name: security
    watches: main
    prompt: "Review"
`)
			// First run processes
			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			out, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "first run: %s", string(out))

			// Second run should be idle (nothing new)
			cmd2 := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			out2, err := cmd2.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "second run: %s", string(out2))
		})

		It("writes idle status without result", func() {
			s := readStatus("security")
			Expect(s.State).To(Equal("idle"))
		})
	})

	Context("when a concern is skipped due to upstream failure", func() {
		BeforeEach(func() {
			configPath = filepath.Join(repoDir, "line.yaml")
			writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "exit 1"]

concerns:
  - name: security
    watches: main
    prompt: "This will fail"
  - name: docs
    watches: security
    prompt: "This should be skipped"
`)
			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			cmd.CombinedOutput() // ignore exit code; concern failures are logged but don't stop the run
		})

		It("marks the failed concern as failed", func() {
			s := readStatus("security")
			Expect(s.State).To(Equal("failed"))
			Expect(s.Error).NotTo(BeEmpty())
		})

		It("marks the downstream concern as skipped", func() {
			s := readStatus("docs")
			Expect(s.State).To(Equal("skipped"))
		})
	})
})
