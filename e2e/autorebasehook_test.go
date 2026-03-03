package e2e_test

import (
	"encoding/json"

	lineGit "github.com/re-cinq/assembly-line/internal/git"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("line auto-rebase-hook", func() {
	var dir string

	BeforeEach(func() {
		dir = tempRepo()
	})

	writeAutoRebaseConfig := func(dir string, autoRebase bool) {
		ar := "false"
		if autoRebase {
			ar = "true"
		}
		writeConfig(dir, `agent:
  command: echo
  args: ["hello"]

settings:
  watches: master
  auto_rebase: `+ar+`

stations:
  - name: review
    prompt: "Review code"
  - name: cleanup
    prompt: "Clean up code"
`)
	}

	// setupStationBranches creates station branches with known commits.
	// Same pattern as rebase_test.go.
	setupStationBranches := func(dir string) {
		writeFile(dir, "code.go", "package main\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add code")

		reviewBranch := lineGit.StationBranchName("review")
		git(dir, "branch", reviewBranch, "master")
		git(dir, "checkout", reviewBranch)
		writeFile(dir, "review.txt", "reviewed\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "[skip line] station review")

		cleanupBranch := lineGit.StationBranchName("cleanup")
		git(dir, "branch", cleanupBranch, reviewBranch)
		git(dir, "checkout", cleanupBranch)
		writeFile(dir, "cleanup.txt", "cleaned\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "[skip line] station cleanup")

		git(dir, "checkout", "master")
	}

	It("outputs block JSON when auto_rebase is true and changes are ready [HOOK-1]", func() {
		writeAutoRebaseConfig(dir, true)
		setupStationBranches(dir)

		out := lineOK(dir, "auto-rebase-hook")

		var result map[string]string
		Expect(json.Unmarshal([]byte(out), &result)).To(Succeed())
		Expect(result["decision"]).To(Equal("block"))
		Expect(result["reason"]).To(ContainSubstring("Auto-rebased"))
		Expect(result["reason"]).To(ContainSubstring("review.txt"))
		Expect(result["reason"]).To(ContainSubstring("cleanup.txt"))
	})

	It("exits silently when auto_rebase is false [HOOK-3]", func() {
		writeAutoRebaseConfig(dir, false)
		setupStationBranches(dir)

		out := lineOK(dir, "auto-rebase-hook")
		Expect(out).To(BeEmpty())
	})

	It("exits silently when no changes to pick up [HOOK-3]", func() {
		writeAutoRebaseConfig(dir, true)
		// No station branches created — nothing to rebase

		out := lineOK(dir, "auto-rebase-hook")
		Expect(out).To(BeEmpty())
	})

	It("dedup: silent on second call for same terminal ref [HOOK-2]", func() {
		writeAutoRebaseConfig(dir, true)
		setupStationBranches(dir)

		out1 := lineOK(dir, "auto-rebase-hook")
		Expect(out1).NotTo(BeEmpty())

		out2 := lineOK(dir, "auto-rebase-hook")
		Expect(out2).To(BeEmpty())
	})

	It("re-fires when terminal ref advances [HOOK-2]", func() {
		writeAutoRebaseConfig(dir, true)
		setupStationBranches(dir)

		out1 := lineOK(dir, "auto-rebase-hook")
		Expect(out1).NotTo(BeEmpty())

		// Advance terminal station with a new commit.
		cleanupBranch := lineGit.StationBranchName("cleanup")
		git(dir, "checkout", cleanupBranch)
		writeFile(dir, "extra.txt", "extra\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "[skip line] more cleanup")
		git(dir, "checkout", "master")

		out2 := lineOK(dir, "auto-rebase-hook")
		var result map[string]string
		Expect(json.Unmarshal([]byte(out2), &result)).To(Succeed())
		Expect(result["decision"]).To(Equal("block"))
		Expect(result["reason"]).To(ContainSubstring("extra.txt"))
	})

	It("exits silently when no config exists [HOOK-3]", func() {
		// No line.yaml in the repo
		out := lineOK(dir, "auto-rebase-hook")
		Expect(out).To(BeEmpty())
	})
})
