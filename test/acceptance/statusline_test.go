package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("line statusline", func() {
	var tmpDir string
	var repoDir string

	BeforeEach(func() {
		tmpDir, repoDir = setupTestRepo("line-statusline-render-*")
	})

	AfterEach(func() {
		cleanupTestRepo(repoDir, tmpDir)
	})

	Context("with a line config (no forks)", func() {
		BeforeEach(func() {
			writeFile(filepath.Join(repoDir, "line.yaml"), `
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
`)
		})

		It("renders a simple line with cwd input", func() {
			cmd := exec.Command(binaryPath, "statusline")
			cmd.Stdin = strings.NewReader(`{"cwd":"` + repoDir + `"}`)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "statusline failed: %s", string(output))

			text := stripANSI(string(output))
			Expect(text).To(ContainSubstring("main"))
			Expect(text).To(ContainSubstring("security"))
			Expect(text).To(ContainSubstring("docs"))
			// Simple line uses ─── connector
			Expect(text).To(ContainSubstring("───"))
			// Line uses ── between stations
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
			writeFile(filepath.Join(repoDir, "line.yaml"), `
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

		It("renders tree connectors for forks", func() {
			cmd := exec.Command(binaryPath, "statusline")
			cmd.Stdin = strings.NewReader(`{"cwd":"` + repoDir + `"}`)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "statusline failed: %s", string(output))

			text := stripANSI(string(output))
			Expect(text).To(ContainSubstring("─┬─"))
			Expect(text).To(ContainSubstring("└─"))
		})

		It("shows unknown state symbols for never-processed stations", func() {
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

	Context("with a non-line directory", func() {
		It("exits silently with no output", func() {
			nonLineDir, err := os.MkdirTemp("", "not-line-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(nonLineDir)

			cmd := exec.Command(binaryPath, "statusline")
			cmd.Stdin = strings.NewReader(`{"cwd":"` + nonLineDir + `"}`)
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
			writeFile(filepath.Join(repoDir, "line.yaml"), `
agent:
  command: "sh"
  args: ["-c", "echo 'reviewed' > agent-review.txt"]

stations:
  - name: security
    watches: main
    prompt: "Security review"
`)
			cmd := exec.Command(binaryPath, "run", "--once", "--path", filepath.Join(repoDir, "line.yaml"))
			out, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "run failed: %s", string(out))
		})

		It("shows modified state symbol", func() {
			cmd := exec.Command(binaryPath, "statusline")
			cmd.Stdin = strings.NewReader(`{"cwd":"` + repoDir + `"}`)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			text := stripANSI(string(output))
			// When caught up (idle), shows ✓ regardless of whether modifications were produced
			Expect(text).To(ContainSubstring("security ✓"))
		})
	})

	Context("after a noop run (caught up)", func() {
		BeforeEach(func() {
			writeFile(filepath.Join(repoDir, "line.yaml"), `
agent:
  command: "true"

stations:
  - name: security
    watches: main
    prompt: "Security review"
`)
			cmd := exec.Command(binaryPath, "run", "--once", "--path", filepath.Join(repoDir, "line.yaml"))
			out, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "run failed: %s", string(out))
		})

		It("shows checkmark for caught-up station", func() {
			cmd := exec.Command(binaryPath, "statusline")
			cmd.Stdin = strings.NewReader(`{"cwd":"` + repoDir + `"}`)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			text := stripANSI(string(output))
			Expect(text).To(ContainSubstring("security ✓"))
		})
	})

	Context("when a station is behind HEAD (pending)", func() {
		BeforeEach(func() {
			writeFile(filepath.Join(repoDir, "line.yaml"), `
agent:
  command: "true"

stations:
  - name: security
    watches: main
    prompt: "Security review"
`)
			cmd := exec.Command(binaryPath, "run", "--once", "--path", filepath.Join(repoDir, "line.yaml"))
			out, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "run failed: %s", string(out))

			// Add a new commit
			writeFile(filepath.Join(repoDir, "new.txt"), "new\n")
			runGit(repoDir, "add", "new.txt")
			runGit(repoDir, "commit", "-m", "new commit")
		})

		It("shows pending symbol for behind-HEAD station", func() {
			cmd := exec.Command(binaryPath, "statusline")
			cmd.Stdin = strings.NewReader(`{"cwd":"` + repoDir + `"}`)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			text := stripANSI(string(output))
			Expect(text).To(ContainSubstring("security ◯"))
		})
	})

	Context("config discovery walks up directories", func() {
		It("finds config in parent directory", func() {
			writeFile(filepath.Join(repoDir, "line.yaml"), `
agent:
  command: "sh"
  args: ["-c", "echo noop"]

stations:
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
