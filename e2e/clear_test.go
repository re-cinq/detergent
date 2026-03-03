package e2e_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	lineGit "github.com/re-cinq/assembly-line/internal/git"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("line clear", func() {
	var dir string

	BeforeEach(func() {
		dir = tempRepo()
	})

	writeClearConfig := func(dir, agentPath string) {
		writeConfig(dir, `agent:
  command: `+agentPath+`
  args: ["-p"]

settings:
  watches: master

stations:
  - name: review
    prompt: "Review code"
  - name: cleanup
    prompt: "Clean up code"
`)
	}

	It("removes station branches [CLEAR-1]", func() {
		agentScript := writeMockAgent(dir)
		writeClearConfig(dir, agentScript)
		installHooksForTest(dir)

		writeFile(dir, "code.go", "package main\n")
		gitCommit(dir, "add code")

		// Verify branches exist
		Expect(git(dir, "branch")).To(ContainSubstring("line/stn/review"))
		Expect(git(dir, "branch")).To(ContainSubstring("line/stn/cleanup"))

		lineOK(dir, "clear", "--force")

		// Verify branches are gone
		branches := git(dir, "branch")
		Expect(branches).NotTo(ContainSubstring("line/stn/review"))
		Expect(branches).NotTo(ContainSubstring("line/stn/cleanup"))
	})

	It("removes .line state directory [CLEAR-1]", func() {
		failingAgent := writeFailingMockAgent(dir)
		writeConfig(dir, `agent:
  command: `+failingAgent+`
  args: ["-p"]

settings:
  watches: master

stations:
  - name: review
    prompt: "Review code"
`)
		installHooksForTest(dir)

		writeFile(dir, "code.go", "package main\n")
		gitCommit(dir, "add code")

		// A failed station leaves a .failed marker in .line/stations/
		Expect(fileExists(dir, ".line/stations/review.failed")).To(BeTrue())

		lineOK(dir, "clear", "--force")

		// .line/stations/ should be gone
		Expect(fileExists(dir, ".line/stations")).To(BeFalse())
	})

	It("terminates running agents [CLEAR-1]", func() {
		slowAgent := writeSlowMockAgent(dir)
		writeConfig(dir, `agent:
  command: `+slowAgent+`
  args: ["-p"]

settings:
  watches: master

stations:
  - name: slow
    prompt: "Be slow"
`)
		installHooksForTestBg(dir)

		writeFile(dir, "code.go", "package main\n")
		gitCommit(dir, "add code")

		// Wait for station agent to start
		Eventually(func() bool {
			return fileExists(dir, ".line/stations/slow.pid")
		}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

		// Read station agent PID
		pidContent := readFile(dir, ".line/stations/slow.pid")
		parts := strings.SplitN(strings.TrimSpace(pidContent), " ", 2)
		var agentPID int
		fmt.Sscanf(parts[0], "%d", &agentPID)
		Expect(agentPID).To(BeNumerically(">", 0))
		Expect(syscall.Kill(agentPID, 0)).To(Succeed(), "agent should be alive before clear")

		DeferCleanup(func() { killBackground(dir, "slow") })

		lineOK(dir, "clear", "--force")

		Eventually(func() error {
			return syscall.Kill(agentPID, 0)
		}, 5*time.Second, 100*time.Millisecond).ShouldNot(Succeed(),
			"agent (PID %d) should have been killed by clear", agentPID)
	})

	It("removes worktrees [CLEAR-1]", func() {
		slowAgent := writeSlowMockAgent(dir)
		writeConfig(dir, `agent:
  command: `+slowAgent+`
  args: ["-p"]

settings:
  watches: master

stations:
  - name: slow
    prompt: "Be slow"
`)
		installHooksForTestBg(dir)

		writeFile(dir, "code.go", "package main\n")
		gitCommit(dir, "add code")

		// Wait for station agent to start (running in worktree)
		Eventually(func() bool {
			return fileExists(dir, ".line/stations/slow.pid")
		}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

		DeferCleanup(func() { killBackground(dir, "slow") })

		// Verify worktree exists
		baseDir, err := lineGit.WorktreeBaseDir(dir)
		Expect(err).NotTo(HaveOccurred())
		wtPath := filepath.Join(baseDir, "slow")
		Expect(wtPath).To(BeADirectory())

		lineOK(dir, "clear", "--force")

		// Worktree should be gone
		_, err = os.Stat(baseDir)
		Expect(os.IsNotExist(err)).To(BeTrue(),
			"worktree base dir %s should not exist after clear", baseDir)
	})

	// HOOK-4: clear removes the rebase-prompted marker
	It("removes rebase-prompted marker [HOOK-4]", func() {
		agentScript := writeMockAgent(dir)
		writeClearConfig(dir, agentScript)

		// Create a rebase-prompted marker
		writeFile(dir, ".line/rebase-prompted", "abc123")
		Expect(fileExists(dir, ".line/rebase-prompted")).To(BeTrue())

		lineOK(dir, "clear", "--force")

		Expect(fileExists(dir, ".line/rebase-prompted")).To(BeFalse())
	})

	It("is safe when line has never run [CLEAR-1]", func() {
		agentScript := writeMockAgent(dir)
		writeClearConfig(dir, agentScript)

		// No run has happened — clear should succeed as a no-op
		lineOK(dir, "clear", "--force")
	})

	It("is idempotent [CLEAR-1]", func() {
		agentScript := writeMockAgent(dir)
		writeClearConfig(dir, agentScript)
		installHooksForTest(dir)

		writeFile(dir, "code.go", "package main\n")
		gitCommit(dir, "add code")

		lineOK(dir, "clear", "--force")
		lineOK(dir, "clear", "--force")
	})

	It("requires confirmation without --force [CLEAR-2]", func() {
		agentScript := writeMockAgent(dir)
		writeClearConfig(dir, agentScript)

		// Without --force and without a tty, should fail
		_, err := line(dir, "clear")
		Expect(err).To(HaveOccurred())
	})

	It("proceeds with --force [CLEAR-2]", func() {
		agentScript := writeMockAgent(dir)
		writeClearConfig(dir, agentScript)

		out := lineOK(dir, "clear", "--force")
		Expect(out).To(ContainSubstring("cleared"))
	})
})
