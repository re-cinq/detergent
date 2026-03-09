package e2e_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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

	writeAutoResolveConfig := func(dir string) {
		writeConfig(dir, `agent:
  command: echo
  args: ["hello"]

settings:
  watches: master
  auto_rebase: true
  auto_resolve: true

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

	It("exits silently when a line run is in progress [HOOK-3]", func() {
		writeAutoRebaseConfig(dir, true)
		setupStationBranches(dir)

		// Simulate an active runner by writing a PID file with our own PID.
		writeFile(dir, ".line/run.pid", fmt.Sprintf("%d", os.Getpid()))

		out := lineOK(dir, "auto-rebase-hook")
		Expect(out).To(BeEmpty())
	})

	It("exits silently when no config exists [HOOK-3]", func() {
		// No line.yaml in the repo
		out := lineOK(dir, "auto-rebase-hook")
		Expect(out).To(BeEmpty())
	})

	// setupConflict creates station branches and then introduces a conflicting
	// file on both master and the terminal station branch.
	setupConflict := func(dir string) {
		setupStationBranches(dir)

		// Station modifies conflict.txt on the terminal branch.
		cleanupBranch := lineGit.StationBranchName("cleanup")
		git(dir, "checkout", cleanupBranch)
		writeFile(dir, "conflict.txt", "station version\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "[skip line] add conflict.txt")
		git(dir, "checkout", "master")

		// Master also adds conflict.txt with different content.
		writeFile(dir, "conflict.txt", "master version\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add conflict.txt on master")
	}

	It("auto_resolve: true outputs block JSON with conflicted file list [HOOK-1]", func() {
		writeAutoResolveConfig(dir)
		setupConflict(dir)

		out := lineOK(dir, "auto-rebase-hook")

		var result map[string]string
		Expect(json.Unmarshal([]byte(out), &result)).To(Succeed())
		Expect(result["decision"]).To(Equal("block"))
		Expect(result["reason"]).To(ContainSubstring("conflict.txt"))
		Expect(result["reason"]).To(ContainSubstring("git add"))
		Expect(result["reason"]).To(ContainSubstring("git rebase --continue"))
	})

	It("auto_resolve: false aborts cleanly on conflict [HOOK-1]", func() {
		writeAutoRebaseConfig(dir, true)
		setupConflict(dir)

		headBefore := git(dir, "rev-parse", "HEAD")
		out := lineOK(dir, "auto-rebase-hook")

		var result map[string]string
		Expect(json.Unmarshal([]byte(out), &result)).To(Succeed())
		Expect(result["decision"]).To(Equal("block"))
		Expect(result["reason"]).To(ContainSubstring("conflict"))

		// HEAD unchanged — rebase was aborted.
		Expect(git(dir, "rev-parse", "HEAD")).To(Equal(headBefore))

		// No lingering rebase state.
		Expect(filepath.Join(dir, ".git", "rebase-merge")).NotTo(BeADirectory())
		Expect(filepath.Join(dir, ".git", "rebase-apply")).NotTo(BeADirectory())
	})

	It("dedup marker is written even when conflicts are left [HOOK-2]", func() {
		writeAutoResolveConfig(dir)
		setupConflict(dir)

		out1 := lineOK(dir, "auto-rebase-hook")
		Expect(out1).NotTo(BeEmpty())

		// Abort the rebase so we can test dedup (git won't allow another rebase
		// while one is in progress, but dedup should fire first).
		git(dir, "rebase", "--abort")

		// Second call for the same terminal ref should be silent (dedup).
		out2 := lineOK(dir, "auto-rebase-hook")
		Expect(out2).To(BeEmpty())
	})
})
