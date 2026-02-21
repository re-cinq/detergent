package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("line run (self-retiring runner)", func() {
	var tmpDir string
	var repoDir string
	var configPath string

	BeforeEach(func() {
		tmpDir, repoDir = setupTestRepo("line-runner-*")

		configPath = filepath.Join(repoDir, "line.yaml")
		writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "echo 'reviewed' > review.txt"]

settings:

stations:
  - name: security
    watches: main
    prompt: "Review for security issues"
`)
	})

	AfterEach(func() {
		cleanupTestRepo(repoDir, tmpDir)
	})

	It("processes initial commit then exits on its own", func() {
		// Write a trigger file so the runner has work to do
		lineDir := filepath.Join(repoDir, ".line")
		Expect(os.MkdirAll(lineDir, 0o755)).To(Succeed())
		head := strings.TrimSpace(runGitOutput(repoDir, "rev-parse", "HEAD"))
		writeFile(filepath.Join(lineDir, "trigger"), head+"\n")

		cmd := exec.Command(binaryPath, "run", "--path", configPath)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		var outputBuf strings.Builder
		cmd.Stdout = &outputBuf
		cmd.Stderr = &outputBuf

		err := cmd.Start()
		Expect(err).NotTo(HaveOccurred())

		// The runner should process and then self-retire after grace period
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()

		select {
		case err := <-done:
			Expect(err).NotTo(HaveOccurred(), "runner output: %s", outputBuf.String())
		case <-time.After(30 * time.Second):
			cmd.Process.Kill()
			Fail("runner did not exit within timeout; output: " + outputBuf.String())
		}

		// Verify it processed: output branch should exist
		branchOut := runGitOutput(repoDir, "branch", "--list", "line/security")
		Expect(branchOut).To(ContainSubstring("line/security"))

		// Verify PID file is cleaned up
		_, err = os.Stat(filepath.Join(repoDir, ".line", "runner.pid"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "PID file should be removed after exit")

		// Verify output mentions exit
		Expect(outputBuf.String()).To(ContainSubstring("exiting"))
	})

	It("stays alive when new trigger arrives during grace period", func() {
		lineDir := filepath.Join(repoDir, ".line")
		Expect(os.MkdirAll(lineDir, 0o755)).To(Succeed())
		head := strings.TrimSpace(runGitOutput(repoDir, "rev-parse", "HEAD"))
		writeFile(filepath.Join(lineDir, "trigger"), head+"\n")

		cmd := exec.Command(binaryPath, "run", "--path", configPath)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		var outputBuf strings.Builder
		cmd.Stdout = &outputBuf
		cmd.Stderr = &outputBuf

		err := cmd.Start()
		Expect(err).NotTo(HaveOccurred())

		// Wait a moment, then make a new commit and update the trigger
		time.Sleep(2 * time.Second)

		writeFile(filepath.Join(repoDir, "new-file.txt"), "new content\n")
		runGit(repoDir, "add", "new-file.txt")
		runGit(repoDir, "commit", "-m", "second commit")

		newHead := strings.TrimSpace(runGitOutput(repoDir, "rev-parse", "HEAD"))
		writeFile(filepath.Join(lineDir, "trigger"), newHead+"\n")

		// Wait for the runner to finish — it should process the second commit too
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()

		select {
		case err := <-done:
			Expect(err).NotTo(HaveOccurred(), "runner output: %s", outputBuf.String())
		case <-time.After(30 * time.Second):
			cmd.Process.Kill()
			Fail("runner did not exit within timeout; output: " + outputBuf.String())
		}

		// Both commits should have been processed
		commitCount := runGitOutput(repoDir, "rev-list", "--count", "line/security")
		count := strings.TrimSpace(commitCount)
		Expect(count).NotTo(Equal("1"), "expected runner to process additional commits")
	})

	It("duplicate runner exits immediately", func() {
		lineDir := filepath.Join(repoDir, ".line")
		Expect(os.MkdirAll(lineDir, 0o755)).To(Succeed())
		head := strings.TrimSpace(runGitOutput(repoDir, "rev-parse", "HEAD"))
		writeFile(filepath.Join(lineDir, "trigger"), head+"\n")

		// Start the first runner
		cmd1 := exec.Command(binaryPath, "run", "--path", configPath)
		cmd1.Dir = repoDir
		cmd1.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		var out1 strings.Builder
		cmd1.Stdout = &out1
		cmd1.Stderr = &out1
		Expect(cmd1.Start()).To(Succeed())

		// Give it time to write PID
		time.Sleep(500 * time.Millisecond)

		// Start a second runner — should exit immediately
		cmd2 := exec.Command(binaryPath, "run", "--path", configPath)
		cmd2.Dir = repoDir
		var out2 strings.Builder
		cmd2.Stdout = &out2
		cmd2.Stderr = &out2

		err := cmd2.Run()
		Expect(err).NotTo(HaveOccurred(), "second runner output: %s", out2.String())
		Expect(out2.String()).To(ContainSubstring("already active"))

		// Wait for first runner to finish naturally
		done := make(chan error, 1)
		go func() { done <- cmd1.Wait() }()
		select {
		case err := <-done:
			Expect(err).NotTo(HaveOccurred(), "first runner output: %s", out1.String())
		case <-time.After(30 * time.Second):
			cmd1.Process.Kill()
			Fail("first runner did not exit; output: " + out1.String())
		}
	})
})
