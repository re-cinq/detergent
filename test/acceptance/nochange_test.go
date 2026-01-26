package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("no-change reviews", func() {
	var tmpDir string
	var repoDir string
	var configPath string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "detergent-nochange-*")
		Expect(err).NotTo(HaveOccurred())

		repoDir = filepath.Join(tmpDir, "repo")
		runGit(tmpDir, "init", repoDir)
		runGit(repoDir, "checkout", "-b", "main")
		writeFile(filepath.Join(repoDir, "hello.txt"), "hello\n")
		runGit(repoDir, "add", "hello.txt")
		runGit(repoDir, "commit", "-m", "initial commit")

		// Agent that does nothing (no file changes)
		configPath = filepath.Join(repoDir, "detergent.yaml")
		writeFile(configPath, `
agent:
  command: "true"

concerns:
  - name: security
    watches: main
    prompt: "Review for security issues"
`)
	})

	AfterEach(func() {
		exec.Command("git", "-C", repoDir, "worktree", "prune").Run()
		os.RemoveAll(tmpDir)
	})

	It("fast-forwards the output branch to match upstream", func() {
		cmd := exec.Command(binaryPath, "run", "--once", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// The output branch should exist and point to the same commit as main
		mainHead := strings.TrimSpace(runGitOutput(repoDir, "rev-parse", "main"))
		secHead := strings.TrimSpace(runGitOutput(repoDir, "rev-parse", "detergent/security"))
		Expect(secHead).To(Equal(mainHead))
	})

	It("adds a git note with the review marker", func() {
		cmd := exec.Command(binaryPath, "run", "--once", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// Check the git note on the initial commit
		mainHead := strings.TrimSpace(runGitOutput(repoDir, "rev-parse", "main"))
		noteCmd := exec.Command("git", "notes", "--ref=detergent", "show", mainHead)
		noteCmd.Dir = repoDir
		noteOut, err := noteCmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "note output: %s", string(noteOut))
		Expect(string(noteOut)).To(ContainSubstring("[SECURITY] Reviewed, no changes needed"))
	})

	It("allows downstream concerns to see the branch advance", func() {
		// Config with chain: security (no-change) -> docs
		writeFile(configPath, `
agent:
  command: "true"

concerns:
  - name: security
    watches: main
    prompt: "Review for security issues"
  - name: docs
    watches: security
    prompt: "Update documentation"
`)
		cmd := exec.Command(binaryPath, "run", "--once", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// Both branches should exist (docs could process because security fast-forwarded)
		Expect(runGitOutput(repoDir, "branch")).To(ContainSubstring("detergent/security"))
		Expect(runGitOutput(repoDir, "branch")).To(ContainSubstring("detergent/docs"))
	})
})
