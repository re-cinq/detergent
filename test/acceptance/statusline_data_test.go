package acceptance_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type statuslineOutput struct {
	Concerns []statuslineConcern `json:"concerns"`
	Roots    []string            `json:"roots"`
	Graph    []graphEdge         `json:"graph"`
}

type statuslineConcern struct {
	Name       string `json:"name"`
	Watches    string `json:"watches"`
	State      string `json:"state"`
	LastResult string `json:"last_result,omitempty"`
	HeadCommit string `json:"head_commit,omitempty"`
	LastSeen   string `json:"last_seen,omitempty"`
	Error      string `json:"error,omitempty"`
	BehindHead bool   `json:"behind_head"`
}

type graphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

var _ = Describe("detergent statusline-data", func() {
	var tmpDir string
	var repoDir string
	var configPath string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "detergent-statusline-*")
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
	})

	Context("with a chain config before any run", func() {
		BeforeEach(func() {
			configPath = filepath.Join(repoDir, "detergent.yaml")
			writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "echo noop"]

concerns:
  - name: security
    watches: main
    prompt: "Security review"
  - name: docs
    watches: security
    prompt: "Docs review"
  - name: style
    watches: main
    prompt: "Style review"
`)
		})

		It("outputs valid JSON with all concerns", func() {
			cmd := exec.Command(binaryPath, "statusline-data", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "statusline-data failed: %s", string(output))

			var result statuslineOutput
			Expect(json.Unmarshal(output, &result)).To(Succeed())
			Expect(result.Concerns).To(HaveLen(3))
		})

		It("identifies root concerns correctly", func() {
			cmd := exec.Command(binaryPath, "statusline-data", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			var result statuslineOutput
			Expect(json.Unmarshal(output, &result)).To(Succeed())
			Expect(result.Roots).To(ConsistOf("security", "style"))
		})

		It("includes graph edges", func() {
			cmd := exec.Command(binaryPath, "statusline-data", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			var result statuslineOutput
			Expect(json.Unmarshal(output, &result)).To(Succeed())
			Expect(result.Graph).To(HaveLen(1))
			Expect(result.Graph[0].From).To(Equal("security"))
			Expect(result.Graph[0].To).To(Equal("docs"))
		})

		It("shows unknown state for never-processed concerns", func() {
			cmd := exec.Command(binaryPath, "statusline-data", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			var result statuslineOutput
			Expect(json.Unmarshal(output, &result)).To(Succeed())
			for _, c := range result.Concerns {
				Expect(c.State).To(Equal("unknown"))
			}
		})
	})

	Context("after a successful run", func() {
		BeforeEach(func() {
			configPath = filepath.Join(repoDir, "detergent.yaml")
			writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "echo 'reviewed' > agent-review.txt"]

concerns:
  - name: security
    watches: main
    prompt: "Security review"
`)
			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			out, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "run failed: %s", string(out))
		})

		It("shows idle state with modified result", func() {
			cmd := exec.Command(binaryPath, "statusline-data", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			var result statuslineOutput
			Expect(json.Unmarshal(output, &result)).To(Succeed())
			Expect(result.Concerns).To(HaveLen(1))
			Expect(result.Concerns[0].State).To(Equal("idle"))
			Expect(result.Concerns[0].LastResult).To(Equal("modified"))
			Expect(result.Concerns[0].BehindHead).To(BeFalse())
		})

		It("shows behind_head when new commits appear", func() {
			// Add a new commit
			writeFile(filepath.Join(repoDir, "new.txt"), "new content\n")
			runGit(repoDir, "add", "new.txt")
			runGit(repoDir, "commit", "-m", "new commit")

			cmd := exec.Command(binaryPath, "statusline-data", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			var result statuslineOutput
			Expect(json.Unmarshal(output, &result)).To(Succeed())
			Expect(result.Concerns[0].BehindHead).To(BeTrue())
		})
	})

	Context("normalization: idle + caught up + no last_result", func() {
		BeforeEach(func() {
			configPath = filepath.Join(repoDir, "detergent.yaml")
			writeFile(configPath, `
agent:
  command: "true"

concerns:
  - name: security
    watches: main
    prompt: "Security review"
`)
			// Run with "true" agent — sets idle state but no last_result
			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			out, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "run failed: %s", string(out))
		})

		It("normalizes to noop when caught up", func() {
			cmd := exec.Command(binaryPath, "statusline-data", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			var result statuslineOutput
			Expect(json.Unmarshal(output, &result)).To(Succeed())
			Expect(result.Concerns[0].State).To(Equal("idle"))
			Expect(result.Concerns[0].LastResult).To(Equal("noop"))
		})
	})

	Context("normalization: idle + behind HEAD", func() {
		BeforeEach(func() {
			configPath = filepath.Join(repoDir, "detergent.yaml")
			writeFile(configPath, `
agent:
  command: "true"

concerns:
  - name: security
    watches: main
    prompt: "Security review"
`)
			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			out, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "run failed: %s", string(out))

			// Add a new commit so security is behind HEAD
			writeFile(filepath.Join(repoDir, "new.txt"), "new\n")
			runGit(repoDir, "add", "new.txt")
			runGit(repoDir, "commit", "-m", "new commit")
		})

		It("normalizes state to pending", func() {
			cmd := exec.Command(binaryPath, "statusline-data", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			var result statuslineOutput
			Expect(json.Unmarshal(output, &result)).To(Succeed())
			Expect(result.Concerns[0].State).To(Equal("pending"))
		})
	})

	Context("stale PID detection", func() {
		BeforeEach(func() {
			configPath = filepath.Join(repoDir, "detergent.yaml")
			writeFile(configPath, `
agent:
  command: "true"

concerns:
  - name: security
    watches: main
    prompt: "Security review"
`)
			// Write a fake status file with agent_running state and a dead PID
			statusDir := filepath.Join(repoDir, ".detergent", "status")
			Expect(os.MkdirAll(statusDir, 0755)).To(Succeed())
			writeFile(filepath.Join(statusDir, "security.json"),
				`{"state":"agent_running","started_at":"2025-01-01T00:00:00Z","head_at_start":"abc123","last_seen":"","pid":99999}`)
		})

		It("marks stale active state as failed", func() {
			cmd := exec.Command(binaryPath, "statusline-data", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			var result statuslineOutput
			Expect(json.Unmarshal(output, &result)).To(Succeed())
			Expect(result.Concerns[0].State).To(Equal("failed"))
			Expect(result.Concerns[0].Error).To(ContainSubstring("no longer running"))
		})
	})
})
