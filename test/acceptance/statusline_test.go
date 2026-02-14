package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("detergent statusline", func() {
	var tmpDir string
	var repoDir string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "detergent-statusline-render-*")
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
	})

	Context("with a chain config (no forks)", func() {
		BeforeEach(func() {
			writeFile(filepath.Join(repoDir, "detergent.yaml"), `
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
`)
		})

		It("renders a simple chain with cwd input", func() {
			cmd := exec.Command(binaryPath, "statusline")
			cmd.Stdin = strings.NewReader(`{"cwd":"` + repoDir + `"}`)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "statusline failed: %s", string(output))

			text := stripANSI(string(output))
			Expect(text).To(ContainSubstring("main"))
			Expect(text).To(ContainSubstring("security"))
			Expect(text).To(ContainSubstring("docs"))
			// Simple chain uses ─── connector
			Expect(text).To(ContainSubstring("───"))
			// Chain uses ── between concerns
			Expect(text).To(ContainSubstring("──"))
		})

		It("renders with workspace.project_dir input", func() {
			cmd := exec.Command(binaryPath, "statusline")
			cmd.Stdin = strings.NewReader(`{"workspace":{"project_dir":"` + repoDir + `"}}`)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "statusline failed: %s", string(output))

			text := stripANSI(string(output))
			Expect(text).To(ContainSubstring("security"))
			Expect(text).To(ContainSubstring("docs"))
		})
	})

	Context("with a forking config", func() {
		BeforeEach(func() {
			writeFile(filepath.Join(repoDir, "detergent.yaml"), `
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

		It("renders tree connectors for forks", func() {
			cmd := exec.Command(binaryPath, "statusline")
			cmd.Stdin = strings.NewReader(`{"cwd":"` + repoDir + `"}`)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "statusline failed: %s", string(output))

			text := stripANSI(string(output))
			Expect(text).To(ContainSubstring("─┬─"))
			Expect(text).To(ContainSubstring("└─"))
		})

		It("shows unknown state symbols for never-processed concerns", func() {
			cmd := exec.Command(binaryPath, "statusline")
			cmd.Stdin = strings.NewReader(`{"cwd":"` + repoDir + `"}`)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			text := stripANSI(string(output))
			// Unknown state uses · symbol
			Expect(text).To(ContainSubstring("security ·"))
			Expect(text).To(ContainSubstring("docs ·"))
			Expect(text).To(ContainSubstring("style ·"))
		})
	})

	Context("with a non-detergent directory", func() {
		It("exits silently with no output", func() {
			nonDetergentDir, err := os.MkdirTemp("", "not-detergent-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(nonDetergentDir)

			cmd := exec.Command(binaryPath, "statusline")
			cmd.Stdin = strings.NewReader(`{"cwd":"` + nonDetergentDir + `"}`)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(BeEmpty())
		})
	})

	Context("with invalid stdin", func() {
		It("exits silently on empty input", func() {
			cmd := exec.Command(binaryPath, "statusline")
			cmd.Stdin = strings.NewReader("")
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(BeEmpty())
		})

		It("exits silently on invalid JSON", func() {
			cmd := exec.Command(binaryPath, "statusline")
			cmd.Stdin = strings.NewReader("not json")
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(BeEmpty())
		})
	})

	Context("after a successful run", func() {
		BeforeEach(func() {
			writeFile(filepath.Join(repoDir, "detergent.yaml"), `
agent:
  command: "sh"
  args: ["-c", "echo 'reviewed' > agent-review.txt"]

concerns:
  - name: security
    watches: main
    prompt: "Security review"
`)
			cmd := exec.Command(binaryPath, "run", "--once", filepath.Join(repoDir, "detergent.yaml"))
			out, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "run failed: %s", string(out))
		})

		It("shows modified state symbol", func() {
			cmd := exec.Command(binaryPath, "statusline")
			cmd.Stdin = strings.NewReader(`{"cwd":"` + repoDir + `"}`)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			text := stripANSI(string(output))
			// Modified state uses * symbol
			Expect(text).To(ContainSubstring("security *"))
		})
	})

	Context("config discovery walks up directories", func() {
		It("finds config in parent directory", func() {
			writeFile(filepath.Join(repoDir, "detergent.yaml"), `
agent:
  command: "sh"
  args: ["-c", "echo noop"]

concerns:
  - name: security
    watches: main
    prompt: "Security review"
`)
			subDir := filepath.Join(repoDir, "src", "deep")
			Expect(os.MkdirAll(subDir, 0755)).To(Succeed())

			cmd := exec.Command(binaryPath, "statusline")
			cmd.Stdin = strings.NewReader(`{"cwd":"` + subDir + `"}`)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "statusline failed: %s", string(output))

			text := stripANSI(string(output))
			Expect(text).To(ContainSubstring("security"))
		})
	})
})

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' {
			// Skip until we find the terminating letter
			i++
			if i < len(s) && s[i] == '[' {
				i++
				for i < len(s) && !((s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z')) {
					i++
				}
				if i < len(s) {
					i++ // skip the terminating letter
				}
			}
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}
