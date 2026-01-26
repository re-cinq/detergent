package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("concern chaining", func() {
	var tmpDir string
	var repoDir string
	var configPath string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "detergent-chain-*")
		Expect(err).NotTo(HaveOccurred())

		repoDir = filepath.Join(tmpDir, "repo")
		runGit(tmpDir, "init", repoDir)
		runGit(repoDir, "checkout", "-b", "main")
		writeFile(filepath.Join(repoDir, "hello.txt"), "hello\n")
		runGit(repoDir, "add", "hello.txt")
		runGit(repoDir, "commit", "-m", "initial commit")

		// Config with A -> B chain: security watches main, docs watches security
		configPath = filepath.Join(repoDir, "detergent.yaml")
		writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "date +%s%N > agent-output.txt"]

concerns:
  - name: security
    watches: main
    prompt: "Review for security issues"
  - name: docs
    watches: security
    prompt: "Update documentation"
`)
	})

	AfterEach(func() {
		exec.Command("git", "-C", repoDir, "worktree", "prune").Run()
		os.RemoveAll(tmpDir)
	})

	It("processes both concerns in order after a single run --once", func() {
		cmd := exec.Command(binaryPath, "run", "--once", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// Both output branches should exist
		branches := runGitOutput(repoDir, "branch")
		Expect(branches).To(ContainSubstring("detergent/security"))
		Expect(branches).To(ContainSubstring("detergent/docs"))
	})

	It("creates commits on the downstream branch with concern tags", func() {
		cmd := exec.Command(binaryPath, "run", "--once", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// Check security branch has tagged commit
		secMsg := runGitOutput(repoDir, "log", "-1", "--format=%s", "detergent/security")
		Expect(secMsg).To(ContainSubstring("[SECURITY]"))

		// Check docs branch has tagged commit
		docsMsg := runGitOutput(repoDir, "log", "-1", "--format=%s", "detergent/docs")
		Expect(docsMsg).To(ContainSubstring("[DOCS]"))
	})

	It("includes upstream commit info in the downstream context", func() {
		cmd := exec.Command(binaryPath, "run", "--once", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// The docs concern's Triggered-By should reference the security branch commit
		docsBody := runGitOutput(repoDir, "log", "-1", "--format=%B", "detergent/docs")
		Expect(docsBody).To(ContainSubstring("Triggered-By:"))

		// Get the security branch HEAD and verify docs references it
		secHead := strings.TrimSpace(runGitOutput(repoDir, "rev-parse", "detergent/security"))
		Expect(docsBody).To(ContainSubstring(secHead))
	})
})
