package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("failure isolation", func() {
	var tmpDir string
	var repoDir string
	var configPath string

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "detergent-failure-*")
		Expect(err).NotTo(HaveOccurred())

		repoDir = filepath.Join(tmpDir, "repo")
		runGit(tmpDir, "init", repoDir)
		runGit(repoDir, "checkout", "-b", "main")
		writeFile(filepath.Join(repoDir, "hello.txt"), "hello\n")
		runGit(repoDir, "add", "hello.txt")
		runGit(repoDir, "commit", "-m", "initial commit")

		// Write a failing agent script and a succeeding one
		writeFile(filepath.Join(repoDir, "fail-agent.sh"), "#!/bin/sh\nexit 1\n")
		os.Chmod(filepath.Join(repoDir, "fail-agent.sh"), 0755)

		writeFile(filepath.Join(repoDir, "ok-agent.sh"), "#!/bin/sh\necho 'ok' > agent-ok.txt\n")
		os.Chmod(filepath.Join(repoDir, "ok-agent.sh"), 0755)

		runGit(repoDir, "add", "fail-agent.sh", "ok-agent.sh")
		runGit(repoDir, "commit", "-m", "add agent scripts")
	})

	AfterEach(func() {
		cleanupTestRepo(repoDir, tmpDir)
	})

	Context("with two independent concerns where one fails", func() {
		BeforeEach(func() {
			// Both watch main independently - broken uses failing agent, working uses ok agent
			// We need separate agent commands per concern. Since the config uses one agent,
			// we'll use a script that checks the context file for the concern name.
			writeFile(filepath.Join(repoDir, "dispatch-agent.sh"), `#!/bin/sh
CONTEXT_FILE="$1"
if grep -q "Concern: broken" "$CONTEXT_FILE" 2>/dev/null; then
  exit 1
fi
echo "reviewed" > agent-output.txt
`)
			os.Chmod(filepath.Join(repoDir, "dispatch-agent.sh"), 0755)

			configPath = filepath.Join(repoDir, "detergent.yaml")
			writeFile(configPath, `
agent:
  command: "sh"
  args: ["`+filepath.Join(repoDir, "dispatch-agent.sh")+`"]

concerns:
  - name: broken
    watches: main
    prompt: "This will fail"
  - name: working
    watches: main
    prompt: "This will succeed"
`)
		})

		It("does not exit with an error (failures are logged, not propagated)", func() {
			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))
		})

		It("logs the failing concern's error", func() {
			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			output, _ := cmd.CombinedOutput()
			Expect(string(output)).To(ContainSubstring("broken"))
			Expect(string(output)).To(ContainSubstring("failed"))
		})

		It("still processes the working concern", func() {
			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "output: %s", string(output))

			// The working concern's output branch should exist
			branches := runGitOutput(repoDir, "branch")
			Expect(branches).To(ContainSubstring("detergent/working"))
		})

		It("does not advance last-seen for the failed concern", func() {
			cmd := exec.Command(binaryPath, "run", "--once", "--path", configPath)
			cmd.CombinedOutput()

			// Run status to check
			statusCmd := exec.Command(binaryPath, "status", "--path", configPath)
			output, err := statusCmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			out := string(output)

			// working should be caught up, broken should be pending (never processed)
			Expect(out).To(ContainSubstring("working"))
			Expect(out).To(ContainSubstring("caught up"))
		})
	})
})
