package acceptance_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type statuslineOutput struct {
	Stations           []statuslineStation `json:"stations"`
	Roots              []string            `json:"roots"`
	Graph              []graphEdge         `json:"graph"`
	HasUnpickedCommits bool                `json:"has_unpicked_commits"`
}

type statuslineStation struct {
	Name       string `json:"name"`
	Watches    string `json:"watches"`
	State      string `json:"state"`
	LastResult string `json:"last_result,omitempty"`
	HeadCommit string `json:"head_commit,omitempty"`
	Error      string `json:"error,omitempty"`
	BehindHead bool   `json:"behind_head"`
}

type graphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

var _ = Describe("line statusline-data", func() {
	var tmpDir string
	var repoDir string
	var configPath string

	BeforeEach(func() {
		tmpDir, repoDir = setupTestRepo("line-statusline-*")
	})

	AfterEach(func() {
		cleanupTestRepo(repoDir, tmpDir)
	})

	Context("with a line config before any run", func() {
		BeforeEach(func() {
			configPath = filepath.Join(repoDir, "line.yaml")
			writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "echo noop"]

stations:
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

		It("outputs valid JSON with all stations", func() {
			cmd := exec.Command(binaryPath, "statusline-data", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "statusline-data failed: %s", string(output))

			var result statuslineOutput
			Expect(json.Unmarshal(output, &result)).To(Succeed())
			Expect(result.Stations).To(HaveLen(3))
		})

		It("identifies root stations correctly", func() {
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

		It("shows unknown state for never-processed stations", func() {
			cmd := exec.Command(binaryPath, "statusline-data", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			var result statuslineOutput
			Expect(json.Unmarshal(output, &result)).To(Succeed())
			for _, c := range result.Stations {
				Expect(c.State).To(Equal("unknown"))
			}
		})
	})

	Context("after a successful run", func() {
		BeforeEach(func() {
			configPath = filepath.Join(repoDir, "line.yaml")
			writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "echo 'reviewed' > agent-review.txt"]

stations:
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
			Expect(result.Stations).To(HaveLen(1))
			Expect(result.Stations[0].State).To(Equal("idle"))
			Expect(result.Stations[0].LastResult).To(Equal("modified"))
			Expect(result.Stations[0].BehindHead).To(BeFalse())
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
			Expect(result.Stations[0].BehindHead).To(BeTrue())
		})
	})

	Context("normalization: idle + caught up + no last_result", func() {
		BeforeEach(func() {
			configPath = filepath.Join(repoDir, "line.yaml")
			writeFile(configPath, `
agent:
  command: "true"

stations:
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
			Expect(result.Stations[0].State).To(Equal("idle"))
			Expect(result.Stations[0].LastResult).To(Equal("noop"))
		})
	})

	Context("normalization: idle + behind HEAD", func() {
		BeforeEach(func() {
			configPath = filepath.Join(repoDir, "line.yaml")
			writeFile(configPath, `
agent:
  command: "true"

stations:
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
			Expect(result.Stations[0].State).To(Equal("pending"))
		})
	})

	Context("has_unpicked_commits detection", func() {
		BeforeEach(func() {
			configPath = filepath.Join(repoDir, "line.yaml")
			writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "echo 'reviewed' > agent-review.txt"]

stations:
  - name: security
    watches: main
    prompt: "Security review"
`)
			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			out, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "run failed: %s", string(out))
		})

		It("is true when station branch has commits ahead of watched branch", func() {
			cmd := exec.Command(binaryPath, "statusline-data", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			var result statuslineOutput
			Expect(json.Unmarshal(output, &result)).To(Succeed())
			Expect(result.HasUnpickedCommits).To(BeTrue())
		})

		It("renders rebase hint in statusline", func() {
			cmd := exec.Command(binaryPath, "statusline")
			cmd.Stdin = strings.NewReader(`{"cwd":"` + repoDir + `"}`)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			text := stripANSI(string(output))
			Expect(text).To(ContainSubstring("/line-rebase"))
		})

	})

	Context("has_unpicked_commits is false for noop agent", func() {
		BeforeEach(func() {
			configPath = filepath.Join(repoDir, "line.yaml")
			writeFile(configPath, `
agent:
  command: "true"

stations:
  - name: security
    watches: main
    prompt: "Security review"
`)
			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			out, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "run failed: %s", string(out))
		})

		It("is false when station branch has no commits ahead", func() {
			cmd := exec.Command(binaryPath, "statusline-data", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			var result statuslineOutput
			Expect(json.Unmarshal(output, &result)).To(Succeed())
			Expect(result.HasUnpickedCommits).To(BeFalse())
		})
	})

	Context("stale PID detection", func() {
		BeforeEach(func() {
			configPath = filepath.Join(repoDir, "line.yaml")
			writeFile(configPath, `
agent:
  command: "true"

stations:
  - name: security
    watches: main
    prompt: "Security review"
`)
			// Write a fake status file with agent_running state and a dead PID
			statusDir := filepath.Join(repoDir, ".line", "status")
			Expect(os.MkdirAll(statusDir, 0755)).To(Succeed())
			writeFile(filepath.Join(statusDir, "security.json"),
				`{"state":"agent_running","started_at":"2025-01-01T00:00:00Z","head_at_start":"abc123","pid":99999}`)
		})

		It("marks stale active state as failed", func() {
			cmd := exec.Command(binaryPath, "statusline-data", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			var result statuslineOutput
			Expect(json.Unmarshal(output, &result)).To(Succeed())
			Expect(result.Stations[0].State).To(Equal("failed"))
			Expect(result.Stations[0].Error).To(ContainSubstring("no longer running"))
		})
	})
})
