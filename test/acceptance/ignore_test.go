package acceptance_test

import (
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe(".lineignore support", func() {
	var tmpDir string
	var repoDir string

	configFor := func(repoDir string) string {
		p := filepath.Join(repoDir, "line.yaml")
		writeFile(p, `
agent:
  command: "sh"
  args: ["-c", "echo 'reviewed' > agent-review.txt"]

settings:
  branch_prefix: "line/"

stations:
  - name: security
    watches: main
    prompt: "Review for security issues"
`)
		return p
	}

	BeforeEach(func() {
		tmpDir, repoDir = setupTestRepo("line-ignore-*")
	})

	AfterEach(func() {
		cleanupTestRepo(repoDir, tmpDir)
	})

	It("skips commits with only ignored files", func() {
		configPath := configFor(repoDir)

		// First run to establish baseline
		cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "first run: %s", string(out))

		// Get commit count on output branch
		count1 := strings.TrimSpace(runGitOutput(repoDir, "rev-list", "--count", "line/security"))

		// Add .lineignore and commit it (this commit itself should be processed)
		writeFile(filepath.Join(repoDir, ".lineignore"), ".beads/\ndocs/\n")
		runGit(repoDir, "add", ".lineignore")
		runGit(repoDir, "commit", "-m", "add lineignore")

		cmd = exec.Command(binaryPath, "run", "--once", "--path", configPath)
		out, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "second run: %s", string(out))

		count2 := strings.TrimSpace(runGitOutput(repoDir, "rev-list", "--count", "line/security"))
		Expect(count2).NotTo(Equal(count1), "commit adding .lineignore should be processed")

		// Now commit only ignored files
		writeFile(filepath.Join(repoDir, ".beads", "issues.jsonl"), `{"id":"test"}`)
		runGit(repoDir, "add", ".beads/issues.jsonl")
		runGit(repoDir, "commit", "-m", "update beads metadata")

		cmd = exec.Command(binaryPath, "run", "--once", "--path", configPath)
		out, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "third run: %s", string(out))

		// Commit count should NOT have increased — commit was skipped
		count3 := strings.TrimSpace(runGitOutput(repoDir, "rev-list", "--count", "line/security"))
		Expect(count3).To(Equal(count2), "commit with only ignored files should be skipped")
	})

	It("processes commits with mixed ignored and non-ignored files", func() {
		configPath := configFor(repoDir)

		// First run to establish baseline
		cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "first run: %s", string(out))

		// Add .lineignore
		writeFile(filepath.Join(repoDir, ".lineignore"), "docs/\n")
		runGit(repoDir, "add", ".lineignore")
		runGit(repoDir, "commit", "-m", "add lineignore")

		cmd = exec.Command(binaryPath, "run", "--once", "--path", configPath)
		out, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "second run: %s", string(out))

		count2 := strings.TrimSpace(runGitOutput(repoDir, "rev-list", "--count", "line/security"))

		// Commit with mixed files (one ignored, one not)
		writeFile(filepath.Join(repoDir, "docs", "guide.md"), "# Guide\n")
		writeFile(filepath.Join(repoDir, "main.go"), "package main\n")
		runGit(repoDir, "add", "docs/guide.md", "main.go")
		runGit(repoDir, "commit", "-m", "add guide and main")

		cmd = exec.Command(binaryPath, "run", "--once", "--path", configPath)
		out, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "third run: %s", string(out))

		count3 := strings.TrimSpace(runGitOutput(repoDir, "rev-list", "--count", "line/security"))
		Expect(count3).NotTo(Equal(count2), "commit with mixed files should be processed")
	})

	It("always processes commits that modify .lineignore", func() {
		configPath := configFor(repoDir)

		// Add .lineignore that ignores itself
		writeFile(filepath.Join(repoDir, ".lineignore"), ".lineignore\n")
		runGit(repoDir, "add", ".lineignore")
		runGit(repoDir, "commit", "-m", "add lineignore")

		// First run to establish baseline
		cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "first run: %s", string(out))

		count1 := strings.TrimSpace(runGitOutput(repoDir, "rev-list", "--count", "line/security"))

		// Modify .lineignore (sole file in commit)
		writeFile(filepath.Join(repoDir, ".lineignore"), ".lineignore\ndocs/\n")
		runGit(repoDir, "add", ".lineignore")
		runGit(repoDir, "commit", "-m", "update lineignore")

		cmd = exec.Command(binaryPath, "run", "--once", "--path", configPath)
		out, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "second run: %s", string(out))

		count2 := strings.TrimSpace(runGitOutput(repoDir, "rev-list", "--count", "line/security"))
		Expect(count2).NotTo(Equal(count1), "commit modifying .lineignore should always be processed")
	})

	It("behaves normally when no .lineignore exists", func() {
		configPath := configFor(repoDir)

		// First run
		cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "first run: %s", string(out))

		count1 := strings.TrimSpace(runGitOutput(repoDir, "rev-list", "--count", "line/security"))

		// Commit any file — should be processed normally (no .lineignore)
		writeFile(filepath.Join(repoDir, "docs", "guide.md"), "# Guide\n")
		runGit(repoDir, "add", "docs/guide.md")
		runGit(repoDir, "commit", "-m", "add docs")

		cmd = exec.Command(binaryPath, "run", "--once", "--path", configPath)
		out, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "second run: %s", string(out))

		count2 := strings.TrimSpace(runGitOutput(repoDir, "rev-list", "--count", "line/security"))
		Expect(count2).NotTo(Equal(count1), "without .lineignore, all commits should be processed")
	})
})
