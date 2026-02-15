package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("detergent run --once", func() {
	var tmpDir string
	var repoDir string
	var configPath string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "detergent-test-*")
		Expect(err).NotTo(HaveOccurred())

		repoDir = filepath.Join(tmpDir, "repo")

		// Initialize a git repo with an initial commit on main
		runGit(tmpDir, "init", repoDir)
		runGit(repoDir, "checkout", "-b", "main")
		writeFile(filepath.Join(repoDir, "hello.txt"), "hello world\n")
		runGit(repoDir, "add", "hello.txt")
		runGit(repoDir, "commit", "-m", "initial commit")

		// Write the config that uses a simple agent
		configPath = filepath.Join(repoDir, "detergent.yaml")
		writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "echo 'reviewed by agent' > agent-review.txt"]

settings:
  branch_prefix: "detergent/"

concerns:
  - name: security
    watches: main
    prompt: "Review for security issues"
`)
	})

	AfterEach(func() {
		// Clean up worktrees before removing tmpDir
		exec.Command("git", "-C", repoDir, "worktree", "prune").Run()
		os.RemoveAll(tmpDir)
	})

	It("exits with code 0", func() {
		cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))
	})

	It("creates the output branch", func() {
		cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// Check that detergent/security branch exists
		out := runGitOutput(repoDir, "branch", "--list", "detergent/security")
		Expect(out).To(ContainSubstring("detergent/security"))
	})

	It("creates a commit on the output branch with the concern tag", func() {
		cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// Check the latest commit on detergent/security
		msg := runGitOutput(repoDir, "log", "-1", "--format=%s", "detergent/security")
		Expect(msg).To(ContainSubstring("[SECURITY]"))
	})

	It("includes the Triggered-By trailer", func() {
		cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		msg := runGitOutput(repoDir, "log", "-1", "--format=%B", "detergent/security")
		Expect(msg).To(ContainSubstring("Triggered-By:"))
	})

	It("pipes context to agent stdin", func() {
		// Use an agent that reads from stdin and writes it to a file
		stdinConfigPath := filepath.Join(repoDir, "detergent-stdin.yaml")
		writeFile(stdinConfigPath, `
agent:
  command: "sh"
  args: ["-c", "cat > stdin-received.txt"]

settings:
  branch_prefix: "detergent/"

concerns:
  - name: security
    watches: main
    prompt: "Review for security issues"
`)
		cmd := exec.Command(binaryPath, "run", "--once", "--path", stdinConfigPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// Verify the agent received context via stdin by checking the file it wrote
		wtPath := filepath.Join(repoDir, ".detergent", "worktrees", "detergent", "security")
		stdinContent, err := os.ReadFile(filepath.Join(wtPath, "stdin-received.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(stdinContent)).To(ContainSubstring("# Concern: security"))
		Expect(string(stdinContent)).To(ContainSubstring("Review for security issues"))
	})

	It("writes permissions settings to worktree when configured", func() {
		permConfigPath := filepath.Join(repoDir, "detergent-perms.yaml")
		writeFile(permConfigPath, `
agent:
  command: "sh"
  args: ["-c", "cat .claude/settings.json > settings-snapshot.txt"]

settings:
  branch_prefix: "detergent/"

permissions:
  allow:
    - Edit
    - Write
    - "Bash(*)"

concerns:
  - name: security
    watches: main
    prompt: "Review for security issues"
`)
		cmd := exec.Command(binaryPath, "run", "--once", "--path", permConfigPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// The agent captured the settings file - check it was written correctly
		wtPath := filepath.Join(repoDir, ".detergent", "worktrees", "detergent", "security")
		snapshot, err := os.ReadFile(filepath.Join(wtPath, "settings-snapshot.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(snapshot)).To(ContainSubstring(`"allow"`))
		Expect(string(snapshot)).To(ContainSubstring(`"Edit"`))
		Expect(string(snapshot)).To(ContainSubstring(`"Write"`))
		Expect(string(snapshot)).To(ContainSubstring(`"Bash(*)`))
	})

	It("does not write permissions when not configured", func() {
		// Use the default config (no permissions block)
		noPermConfigPath := filepath.Join(repoDir, "detergent-noperm.yaml")
		writeFile(noPermConfigPath, `
agent:
  command: "sh"
  args: ["-c", "test -f .claude/settings.json && echo EXISTS || echo MISSING"]

settings:
  branch_prefix: "detergent/"

concerns:
  - name: security
    watches: main
    prompt: "Review for security issues"
`)
		cmd := exec.Command(binaryPath, "run", "--once", "--path", noPermConfigPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// Verify .claude/settings.json was NOT created in the worktree
		wtPath := filepath.Join(repoDir, ".detergent", "worktrees", "detergent", "security")
		_, err = os.Stat(filepath.Join(wtPath, ".claude", "settings.json"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "settings.json should not exist when permissions not configured")
	})

	It("is idempotent - running twice doesn't create duplicate commits", func() {
		cmd1 := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		out1, err := cmd1.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "first run: %s", string(out1))

		// Get commit count after first run
		count1 := runGitOutput(repoDir, "rev-list", "--count", "detergent/security")

		cmd2 := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		out2, err := cmd2.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "second run: %s", string(out2))

		// Commit count should be the same
		count2 := runGitOutput(repoDir, "rev-list", "--count", "detergent/security")
		Expect(count2).To(Equal(count1))
	})
})

func runGit(dir string, args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "git %v: %s", args, string(out))
}

func runGitOutput(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "git %v: %s", args, string(out))
	return string(out)
}

func writeFile(path, content string) {
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	err = os.WriteFile(path, []byte(content), 0644)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}
