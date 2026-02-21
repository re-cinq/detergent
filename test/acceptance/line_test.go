package acceptance_test

import (
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("station chaining", func() {
	var tmpDir string
	var repoDir string
	var configPath string

	BeforeEach(func() {
		tmpDir, repoDir = setupTestRepo("line-line-*")

		// Config with A -> B line: security watches main, docs watches security
		configPath = filepath.Join(repoDir, "line.yaml")
		writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "date +%s%N > agent-output.txt"]

stations:
  - name: security
    watches: main
    prompt: "Review for security issues"
  - name: docs
    watches: security
    prompt: "Update documentation"
`)
	})

	AfterEach(func() {
		cleanupTestRepo(repoDir, tmpDir)
	})

	It("processes both stations in order after a single run --once", func() {
		cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// Both output branches should exist
		branches := runGitOutput(repoDir, "branch")
		Expect(branches).To(ContainSubstring("line/security"))
		Expect(branches).To(ContainSubstring("line/docs"))
	})

	It("creates commits on the downstream branch with station tags", func() {
		cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// Check security branch has tagged commit
		secMsg := runGitOutput(repoDir, "log", "-1", "--format=%s", "line/security")
		Expect(secMsg).To(ContainSubstring("[SECURITY]"))

		// Check docs branch has tagged commit
		docsMsg := runGitOutput(repoDir, "log", "-1", "--format=%s", "line/docs")
		Expect(docsMsg).To(ContainSubstring("[DOCS]"))
	})

	It("includes upstream commit info in the downstream context", func() {
		cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// The docs station's Triggered-By should reference the security branch commit
		docsBody := runGitOutput(repoDir, "log", "-1", "--format=%B", "line/docs")
		Expect(docsBody).To(ContainSubstring("Triggered-By:"))

		// Get the security branch HEAD and verify docs references it
		secHead := strings.TrimSpace(runGitOutput(repoDir, "rev-parse", "line/security"))
		Expect(docsBody).To(ContainSubstring(secHead))
	})
})
