package acceptance_test

import (
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
		tmpDir, repoDir = setupTestRepo("line-nochange-*")

		// Agent that does nothing (no file changes)
		configPath = filepath.Join(repoDir, "line.yaml")
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
		cleanupTestRepo(repoDir, tmpDir)
	})

	It("advances the output branch to match upstream via rebase", func() {
		cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// The output branch should exist and point to the same commit as main
		mainHead := strings.TrimSpace(runGitOutput(repoDir, "rev-parse", "main"))
		secHead := strings.TrimSpace(runGitOutput(repoDir, "rev-parse", "line/security"))
		Expect(secHead).To(Equal(mainHead))
	})

	It("adds a git note with the review marker", func() {
		cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// Check the git note on the initial commit
		mainHead := strings.TrimSpace(runGitOutput(repoDir, "rev-parse", "main"))
		noteCmd := exec.Command("git", "notes", "--ref=line", "show", mainHead)
		noteCmd.Dir = repoDir
		noteOut, err := noteCmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "note output: %s", string(noteOut))
		Expect(string(noteOut)).To(ContainSubstring("[SECURITY] Reviewed, no changes needed"))
	})

	It("rebases output branch when it has prior commits and upstream advances", func() {
		// First run: use an agent that makes changes
		writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "date +%s%N > agent-output.txt"]

concerns:
  - name: security
    watches: main
    prompt: "Review for security issues"
`)
		cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "first run: %s", string(output))

		// Output branch now has a concern commit on top of main
		secHead1 := strings.TrimSpace(runGitOutput(repoDir, "rev-parse", "line/security"))
		mainHead1 := strings.TrimSpace(runGitOutput(repoDir, "rev-parse", "main"))
		Expect(secHead1).NotTo(Equal(mainHead1), "security branch should have its own commit")

		// Now advance main with a new commit (diverging from the output branch)
		writeFile(filepath.Join(repoDir, "world.txt"), "world\n")
		runGit(repoDir, "add", "world.txt")
		runGit(repoDir, "commit", "-m", "second commit on main")

		// Second run: switch agent back to no-op
		writeFile(configPath, `
agent:
  command: "true"

concerns:
  - name: security
    watches: main
    prompt: "Review for security issues"
`)
		cmd = exec.Command(binaryPath, "run", "--once", "--path", configPath)
		output, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "second run: %s", string(output))

		// The output branch should contain the new main commit (rebase succeeded)
		mainHead2 := strings.TrimSpace(runGitOutput(repoDir, "rev-parse", "main"))
		secLog := runGitOutput(repoDir, "log", "--format=%H", "line/security")
		Expect(secLog).To(ContainSubstring(mainHead2), "output branch should contain latest main commit after rebase")
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
		cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// Both branches should exist (docs could process because security fast-forwarded)
		Expect(runGitOutput(repoDir, "branch")).To(ContainSubstring("line/security"))
		Expect(runGitOutput(repoDir, "branch")).To(ContainSubstring("line/docs"))
	})
})
