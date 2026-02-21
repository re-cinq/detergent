package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("line run --once", func() {
	var tmpDir string
	var repoDir string
	var configPath string

	BeforeEach(func() {
		tmpDir, repoDir = setupTestRepo("line-test-*")

		// Write the config that uses a simple agent
		configPath = filepath.Join(repoDir, "line.yaml")
		writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "echo 'reviewed by agent' > agent-review.txt"]

settings:
  branch_prefix: "line/"

stations:
  - name: security
    watches: main
    prompt: "Review for security issues"
`)
	})

	AfterEach(func() {
		cleanupTestRepo(repoDir, tmpDir)
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

		// Check that line/security branch exists
		out := runGitOutput(repoDir, "branch", "--list", "line/security")
		Expect(out).To(ContainSubstring("line/security"))
	})

	It("creates a commit on the output branch with the station tag", func() {
		cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// Check the latest commit on line/security
		msg := runGitOutput(repoDir, "log", "-1", "--format=%s", "line/security")
		Expect(msg).To(ContainSubstring("[SECURITY]"))
	})

	It("includes the Triggered-By trailer", func() {
		cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		msg := runGitOutput(repoDir, "log", "-1", "--format=%B", "line/security")
		Expect(msg).To(ContainSubstring("Triggered-By:"))
	})

	It("pipes context to agent stdin", func() {
		// Use an agent that reads from stdin and writes it to a file
		stdinConfigPath := filepath.Join(repoDir, "line-stdin.yaml")
		writeFile(stdinConfigPath, `
agent:
  command: "sh"
  args: ["-c", "cat > stdin-received.txt"]

settings:
  branch_prefix: "line/"

stations:
  - name: security
    watches: main
    prompt: "Review for security issues"
`)
		cmd := exec.Command(binaryPath, "run", "--once", "--path", stdinConfigPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// Verify the agent received context via stdin by checking the file it wrote
		wtPath := filepath.Join(repoDir, ".line", "worktrees", "line", "security")
		stdinContent, err := os.ReadFile(filepath.Join(wtPath, "stdin-received.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(stdinContent)).To(ContainSubstring("# Station: security"))
		Expect(string(stdinContent)).To(ContainSubstring("Review for security issues"))
	})

	It("uses default preamble when none configured", func() {
		stdinConfigPath := filepath.Join(repoDir, "line-default-preamble.yaml")
		writeFile(stdinConfigPath, `
agent:
  command: "sh"
  args: ["-c", "cat > stdin-received.txt"]

settings:
  branch_prefix: "line/"

stations:
  - name: security
    watches: main
    prompt: "Review for security issues"
`)
		cmd := exec.Command(binaryPath, "run", "--once", "--path", stdinConfigPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		wtPath := filepath.Join(repoDir, ".line", "worktrees", "line", "security")
		content, err := os.ReadFile(filepath.Join(wtPath, "stdin-received.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(ContainSubstring("You are running non-interactively"))
	})

	It("uses global preamble when configured", func() {
		stdinConfigPath := filepath.Join(repoDir, "line-global-preamble.yaml")
		writeFile(stdinConfigPath, `
agent:
  command: "sh"
  args: ["-c", "cat > stdin-received.txt"]

settings:
  branch_prefix: "line/"

preamble: "You are a custom global agent. Proceed silently."

stations:
  - name: security
    watches: main
    prompt: "Review for security issues"
`)
		cmd := exec.Command(binaryPath, "run", "--once", "--path", stdinConfigPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		wtPath := filepath.Join(repoDir, ".line", "worktrees", "line", "security")
		content, err := os.ReadFile(filepath.Join(wtPath, "stdin-received.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(ContainSubstring("You are a custom global agent"))
		Expect(string(content)).NotTo(ContainSubstring("non-interactively"))
	})

	It("uses per-station preamble over global preamble", func() {
		stdinConfigPath := filepath.Join(repoDir, "line-station-preamble.yaml")
		writeFile(stdinConfigPath, `
agent:
  command: "sh"
  args: ["-c", "cat > stdin-received.txt"]

settings:
  branch_prefix: "line/"

preamble: "Global preamble that should be overridden."

stations:
  - name: security
    watches: main
    prompt: "Review for security issues"
    preamble: "Per-station preamble for security reviews."
`)
		cmd := exec.Command(binaryPath, "run", "--once", "--path", stdinConfigPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		wtPath := filepath.Join(repoDir, ".line", "worktrees", "line", "security")
		content, err := os.ReadFile(filepath.Join(wtPath, "stdin-received.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(ContainSubstring("Per-station preamble for security reviews"))
		Expect(string(content)).NotTo(ContainSubstring("Global preamble that should be overridden"))
	})

	It("writes permissions settings to worktree when configured", func() {
		permConfigPath := filepath.Join(repoDir, "line-perms.yaml")
		writeFile(permConfigPath, `
agent:
  command: "sh"
  args: ["-c", "cat .claude/settings.json > settings-snapshot.txt"]

settings:
  branch_prefix: "line/"

permissions:
  allow:
    - Edit
    - Write
    - "Bash(*)"

stations:
  - name: security
    watches: main
    prompt: "Review for security issues"
`)
		cmd := exec.Command(binaryPath, "run", "--once", "--path", permConfigPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// The agent captured the settings file - check it was written correctly
		wtPath := filepath.Join(repoDir, ".line", "worktrees", "line", "security")
		snapshot, err := os.ReadFile(filepath.Join(wtPath, "settings-snapshot.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(snapshot)).To(ContainSubstring(`"allow"`))
		Expect(string(snapshot)).To(ContainSubstring(`"Edit"`))
		Expect(string(snapshot)).To(ContainSubstring(`"Write"`))
		Expect(string(snapshot)).To(ContainSubstring(`"Bash(*)`))
	})

	It("does not write permissions when not configured", func() {
		// Use the default config (no permissions block)
		noPermConfigPath := filepath.Join(repoDir, "line-noperm.yaml")
		writeFile(noPermConfigPath, `
agent:
  command: "sh"
  args: ["-c", "test -f .claude/settings.json && echo EXISTS || echo MISSING"]

settings:
  branch_prefix: "line/"

stations:
  - name: security
    watches: main
    prompt: "Review for security issues"
`)
		cmd := exec.Command(binaryPath, "run", "--once", "--path", noPermConfigPath)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		// Verify .claude/settings.json was NOT created in the worktree
		wtPath := filepath.Join(repoDir, ".line", "worktrees", "line", "security")
		_, err = os.Stat(filepath.Join(wtPath, ".claude", "settings.json"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "settings.json should not exist when permissions not configured")
	})

	It("strips CLAUDECODE env var from agent environment", func() {
		envConfigPath := filepath.Join(repoDir, "line-env.yaml")
		writeFile(envConfigPath, `
agent:
  command: "sh"
  args: ["-c", "env > env-dump.txt"]

settings:
  branch_prefix: "line/"

stations:
  - name: security
    watches: main
    prompt: "Check env"
`)
		cmd := exec.Command(binaryPath, "run", "--once", "--path", envConfigPath)
		cmd.Env = append(os.Environ(), "CLAUDECODE=some-value")
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

		wtPath := filepath.Join(repoDir, ".line", "worktrees", "line", "security")
		envDump, err := os.ReadFile(filepath.Join(wtPath, "env-dump.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(envDump)).NotTo(ContainSubstring("CLAUDECODE="))
		Expect(string(envDump)).To(ContainSubstring("LINE_AGENT=1"))
	})

	It("is idempotent - running twice doesn't create duplicate commits", func() {
		cmd1 := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		out1, err := cmd1.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "first run: %s", string(out1))

		// Get commit count after first run
		count1 := runGitOutput(repoDir, "rev-list", "--count", "line/security")

		cmd2 := exec.Command(binaryPath, "run", "--once", "--path", configPath)
		out2, err := cmd2.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "second run: %s", string(out2))

		// Commit count should be the same
		count2 := runGitOutput(repoDir, "rev-list", "--count", "line/security")
		Expect(count2).To(Equal(count1))
	})
})
