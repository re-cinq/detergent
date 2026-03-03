package e2e_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("line skill", func() {
	var dir string
	var agentScript string

	BeforeEach(func() {
		dir = tempRepo()
		agentScript = writeMockAgent(dir)
	})

	// SKL-2: When changes are picked onto the main branch, these commits must not trigger the line again
	It("station commits contain [skip line] so they don't retrigger when rebased [SKL-2]", func() {
		writeConfig(dir, `agent:
  command: `+agentScript+`
  args: ["-p"]

settings:
  watches: master

stations:
  - name: review
    prompt: "Review code"
  - name: cleanup
    prompt: "Clean up"
`)
		writeFile(dir, "code.go", "package main\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add code")

		lineOK(dir, "run")

		// Check that ALL station commits have [skip line] markers
		// so when rebased onto the watched branch, they won't retrigger
		git(dir, "checkout", "line/stn/review")
		msg := git(dir, "log", "-1", "--format=%s")
		Expect(msg).To(ContainSubstring("[skip line]"))

		git(dir, "checkout", "line/stn/cleanup")
		msg = git(dir, "log", "-1", "--format=%s")
		Expect(msg).To(ContainSubstring("[skip line]"))

		// Simulate what /line-rebase would do: rebase master onto terminal station
		git(dir, "checkout", "master")
		terminalBranch := "line/stn/cleanup"
		git(dir, "rebase", terminalBranch)

		// The last commit on master should still have [skip line]
		msg = git(dir, "log", "-1", "--format=%s")
		Expect(msg).To(ContainSubstring("[skip line]"))

		// Running line run should skip because of the [skip line] marker
		out, err := line(dir, "run")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("skipping"))

		// Intermediate stations should show up to date (not pending)
		statusOut := lineOK(dir, "status")
		Expect(statusOut).NotTo(ContainSubstring("pending"))
	})

	// SKL-1: /line-rebase safely stashes, rebases from terminal, unstashes
	It("rebase procedure: stash, rebase from terminal station, pop stash [SKL-1]", func() {
		writeConfig(dir, `agent:
  command: `+agentScript+`
  args: ["-p"]

settings:
  watches: master

stations:
  - name: review
    prompt: "Review code"
  - name: cleanup
    prompt: "Clean up"
`)
		writeFile(dir, "code.go", "package main\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add code")

		// Run the line so stations have commits
		lineOK(dir, "run")

		// Create WIP (uncommitted staged work) on the watched branch
		writeFile(dir, "wip.txt", "work in progress\n")
		git(dir, "add", "wip.txt")

		// Verify the terminal station has commits ahead of master
		terminalBranch := "line/stn/cleanup"
		countOut := git(dir, "rev-list", "--count", "master.."+terminalBranch)
		Expect(countOut).NotTo(Equal("0"), "terminal station should have commits ahead of master")

		// Execute the rebase procedure as described in SKILL.md:
		// 1. Stash current work
		git(dir, "stash", "push", "-m", "line-rebase: stashing WIP")

		// Verify stash was saved
		stashList := git(dir, "stash", "list")
		Expect(stashList).To(ContainSubstring("line-rebase: stashing WIP"))

		// 2. Rebase from terminal station
		git(dir, "rebase", terminalBranch)

		// 3. Pop stash to restore WIP
		git(dir, "stash", "pop")

		// Verify: master now includes the station improvements
		masterLog := git(dir, "log", "--oneline")
		Expect(masterLog).To(ContainSubstring("assembly-line: station cleanup"))

		// Verify: WIP is restored
		Expect(fileExists(dir, "wip.txt")).To(BeTrue(), "WIP file should be restored after rebase")
		Expect(readFile(dir, "wip.txt")).To(Equal("work in progress\n"))

		// Verify: station branch content is on master
		git(dir, "checkout", terminalBranch)
		Expect(fileExists(dir, "agent-output.txt")).To(BeTrue())

		git(dir, "checkout", "master")
		Expect(fileExists(dir, "agent-output.txt")).To(BeTrue(),
			"master should have agent output after rebase from terminal station")

		// Verify: the rebased commits have [skip line] so they won't retrigger
		out, err := line(dir, "run")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("skipping"))
	})

	// SKL-1: Rebase works even when no WIP exists (stash is a no-op effectively)
	It("rebase procedure works with clean working tree [SKL-1]", func() {
		writeConfig(dir, `agent:
  command: `+agentScript+`
  args: ["-p"]

settings:
  watches: master

stations:
  - name: review
    prompt: "Review code"
`)
		writeFile(dir, "code.go", "package main\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add code")

		lineOK(dir, "run")

		terminalBranch := "line/stn/review"

		// The SKILL.md says "ALWAYS stash before rebase, even if the working tree appears clean"
		// git stash push with a clean tree creates nothing - verify this doesn't break the flow
		stashOut := git(dir, "stash", "push", "-m", "line-rebase: stashing WIP")
		noStash := stashOut == "No local changes to save"

		git(dir, "rebase", terminalBranch)

		if !noStash {
			git(dir, "stash", "pop")
		}

		// Master should now include station work
		Expect(fileExists(dir, "agent-output.txt")).To(BeTrue())
	})

	// SKL-1: Rebase aborts on conflict rather than auto-resolving
	It("rebase procedure aborts on conflict and preserves stash [SKL-1]", func() {
		writeConfig(dir, `agent:
  command: `+agentScript+`
  args: ["-p"]

settings:
  watches: master

stations:
  - name: review
    prompt: "Review code"
`)
		writeFile(dir, "code.go", "package main\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add code")

		lineOK(dir, "run")

		// Create a conflicting change on master that conflicts with station work
		// The agent writes agent-output.txt, so modify it on master too
		writeFile(dir, "agent-output.txt", "conflicting master content\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "create conflict on master")

		// Stash WIP
		writeFile(dir, "wip.txt", "important WIP\n")
		git(dir, "add", "wip.txt")
		git(dir, "stash", "push", "-m", "line-rebase: stashing WIP")

		// Attempt rebase - should fail due to conflict
		terminalBranch := "line/stn/review"
		_, rebaseErr := gitMay(dir, "rebase", terminalBranch)
		if rebaseErr != nil {
			// Abort rebase as the SKILL.md instructs
			git(dir, "rebase", "--abort")
		}

		// Restore stash - WIP should be intact
		git(dir, "stash", "pop")
		Expect(fileExists(dir, "wip.txt")).To(BeTrue(), "WIP should be preserved even after failed rebase")
		Expect(readFile(dir, "wip.txt")).To(Equal("important WIP\n"))
	})

	// SKL-1: Skill file is installed with correct procedure documentation
	It("installed skill file documents the stash/rebase/unstash procedure [SKL-1]", func() {
		lineOK(dir, "init")

		skillPath := filepath.Join(dir, ".claude", "skills", "line-rebase", "SKILL.md")
		_, err := os.Stat(skillPath)
		Expect(err).NotTo(HaveOccurred(), "skill file should be installed")

		content := readFile(dir, filepath.Join(".claude", "skills", "line-rebase", "SKILL.md"))
		Expect(content).To(ContainSubstring("line rebase"))
		Expect(content).To(ContainSubstring("stash"))
		Expect(content).To(ContainSubstring("rebase"))
		Expect(content).To(ContainSubstring("abort"))
		Expect(content).To(ContainSubstring("No work is ever lost"))
	})

	// SKL-3: /line-preview skill is installed by init
	It("init installs the /line-preview skill [SKL-3]", func() {
		lineOK(dir, "init")

		skillPath := filepath.Join(dir, ".claude", "skills", "line-preview", "SKILL.md")
		_, err := os.Stat(skillPath)
		Expect(err).NotTo(HaveOccurred(), "preview skill file should be installed")
	})

	// SKL-3: Installed preview skill contains key instructions
	It("installed preview skill contains read-only git instructions [SKL-3]", func() {
		lineOK(dir, "init")

		content := readFile(dir, filepath.Join(".claude", "skills", "line-preview", "SKILL.md"))
		Expect(content).To(ContainSubstring("git rev-list"))
		Expect(content).To(ContainSubstring("git diff"))
		Expect(content).To(ContainSubstring("read-only"))
	})
})
