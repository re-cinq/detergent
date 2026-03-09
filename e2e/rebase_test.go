package e2e_test

import (
	"os"
	"path/filepath"

	lineGit "github.com/re-cinq/assembly-line/internal/git"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("line rebase", func() {
	var dir string

	BeforeEach(func() {
		dir = tempRepo()
	})

	// setupStationBranches creates station branches with known commits using
	// plain git, independent of `line run`. This makes the test state explicit
	// and avoids coupling to pipeline internals.
	//
	// After setup:
	//   master:             initial → "add code" (code.go)
	//   line/stn/review:    master + commit adding review.txt
	//   line/stn/cleanup:   review + commit adding cleanup.txt
	setupStationBranches := func(dir string) {
		writeConfig(dir, `agent:
  command: echo
  args: ["hello"]

settings:
  watches: master

stations:
  - name: review
    prompt: "Review code"
  - name: cleanup
    prompt: "Clean up code"
`)
		writeFile(dir, "code.go", "package main\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add code")

		// Create review branch from master, add a commit.
		reviewBranch := lineGit.StationBranchName("review")
		git(dir, "branch", reviewBranch, "master")
		git(dir, "checkout", reviewBranch)
		writeFile(dir, "review.txt", "reviewed\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "[skip line] station review")

		// Create cleanup branch from review, add a commit.
		cleanupBranch := lineGit.StationBranchName("cleanup")
		git(dir, "branch", cleanupBranch, reviewBranch)
		git(dir, "checkout", cleanupBranch)
		writeFile(dir, "cleanup.txt", "cleaned\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "[skip line] station cleanup")

		git(dir, "checkout", "master")
	}

	// noRebaseInProgress asserts git has no lingering rebase state.
	noRebaseInProgress := func(dir string) {
		Expect(filepath.Join(dir, ".git", "rebase-apply")).NotTo(BeADirectory())
		Expect(filepath.Join(dir, ".git", "rebase-merge")).NotTo(BeADirectory())
	}

	It("rebases onto terminal station and reports changed files [REB-1, REB-3]", func() {
		setupStationBranches(dir)
		headBefore := git(dir, "rev-parse", "HEAD")

		out := lineOK(dir, "rebase")

		Expect(out).To(ContainSubstring("Rebased onto line/stn/cleanup"))
		Expect(out).To(ContainSubstring("Changed files:"))
		Expect(out).To(ContainSubstring("review.txt"))
		Expect(out).To(ContainSubstring("cleanup.txt"))

		// HEAD must have advanced.
		headAfter := git(dir, "rev-parse", "HEAD")
		Expect(headAfter).NotTo(Equal(headBefore))

		// Station commits are now ancestors of HEAD.
		terminalHead := git(dir, "rev-parse", lineGit.StationBranchName("cleanup"))
		Expect(lineGit.IsAncestor(dir, terminalHead, "HEAD")).To(BeTrue())
	})

	It("reports nothing to do when already up to date [REB-1]", func() {
		setupStationBranches(dir)

		lineOK(dir, "rebase")

		out := lineOK(dir, "rebase")
		Expect(out).To(ContainSubstring("Nothing to rebase"))
	})

	It("reports nothing to do when no station branches exist [REB-1]", func() {
		writeDefaultConfig(dir)

		out := lineOK(dir, "rebase")
		Expect(out).To(ContainSubstring("Nothing to rebase"))
	})

	It("handles clean working tree without stashing [REB-1]", func() {
		setupStationBranches(dir)
		headBefore := git(dir, "rev-parse", "HEAD")

		out := lineOK(dir, "rebase")
		Expect(out).To(ContainSubstring("Rebased onto line/stn/cleanup"))

		headAfter := git(dir, "rev-parse", "HEAD")
		Expect(headAfter).NotTo(Equal(headBefore))

		stashList := git(dir, "stash", "list")
		Expect(stashList).To(BeEmpty())
	})

	It("handles dirty working tree with stash and unstash [REB-1]", func() {
		setupStationBranches(dir)

		// Modify a tracked file as WIP.
		writeFile(dir, "code.go", "package main // WIP\n")

		out := lineOK(dir, "rebase")
		Expect(out).To(ContainSubstring("Rebased onto line/stn/cleanup"))

		// WIP restored.
		Expect(readFile(dir, "code.go")).To(Equal("package main // WIP\n"))

		// Station files arrived.
		Expect(readFile(dir, "review.txt")).To(Equal("reviewed\n"))
		Expect(readFile(dir, "cleanup.txt")).To(Equal("cleaned\n"))

		// No stash entries left behind.
		stashList := git(dir, "stash", "list")
		Expect(stashList).To(BeEmpty())
	})

	It("aborts on rebase conflict and leaves git clean [REB-2]", func() {
		// Station writes one version of conflict.txt; master will write another.
		setupStationBranches(dir)

		// Station modifies conflict.txt on the terminal branch.
		git(dir, "checkout", lineGit.StationBranchName("cleanup"))
		writeFile(dir, "conflict.txt", "station version\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "[skip line] add conflict.txt")
		git(dir, "checkout", "master")

		// Master also adds conflict.txt with different content.
		writeFile(dir, "conflict.txt", "master version\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add conflict.txt on master")

		headBefore := git(dir, "rev-parse", "HEAD")

		out, err := line(dir, "rebase")
		Expect(err).To(HaveOccurred())
		Expect(out).To(ContainSubstring("conflict"))

		// HEAD unchanged — rebase was fully aborted.
		Expect(git(dir, "rev-parse", "HEAD")).To(Equal(headBefore))
		Expect(currentBranch(dir)).To(Equal("master"))
		Expect(readFile(dir, "conflict.txt")).To(Equal("master version\n"))

		// No lingering rebase state.
		noRebaseInProgress(dir)

		// Working tree is clean.
		status, _ := gitMay(dir, "status", "--porcelain")
		Expect(status).To(BeEmpty())
	})

	It("restores stash when rebase conflicts on dirty tree [REB-2]", func() {
		setupStationBranches(dir)

		// Create conflicting state (same as above).
		git(dir, "checkout", lineGit.StationBranchName("cleanup"))
		writeFile(dir, "conflict.txt", "station version\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "[skip line] add conflict.txt")
		git(dir, "checkout", "master")

		writeFile(dir, "conflict.txt", "master version\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add conflict.txt on master")

		// Now dirty the working tree with a tracked modification.
		writeFile(dir, "code.go", "precious WIP\n")

		headBefore := git(dir, "rev-parse", "HEAD")

		_, err := line(dir, "rebase")
		Expect(err).To(HaveOccurred())

		// HEAD unchanged.
		Expect(git(dir, "rev-parse", "HEAD")).To(Equal(headBefore))

		// WIP restored.
		Expect(readFile(dir, "code.go")).To(Equal("precious WIP\n"))

		// No stash entries left behind.
		stashList := git(dir, "stash", "list")
		Expect(stashList).To(BeEmpty())

		// No lingering rebase state.
		noRebaseInProgress(dir)
	})

	It("detects stash pop conflict and leaves stash for manual recovery [REB-2]", func() {
		// Station modifies code.go; we'll also have local WIP on code.go.
		// Rebase will fast-forward (succeed), but stash pop will conflict.
		writeConfig(dir, `agent:
  command: echo
  args: ["hello"]

settings:
  watches: master

stations:
  - name: review
    prompt: "Review code"
`)
		writeFile(dir, "code.go", "original\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add code")

		reviewBranch := lineGit.StationBranchName("review")
		git(dir, "branch", reviewBranch, "master")
		git(dir, "checkout", reviewBranch)
		writeFile(dir, "code.go", "station rewrote this\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "[skip line] station review")
		git(dir, "checkout", "master")

		headBefore := git(dir, "rev-parse", "HEAD")

		// Local WIP on the same file the station modified.
		writeFile(dir, "code.go", "my local WIP\n")

		out, err := line(dir, "rebase")
		Expect(err).To(HaveOccurred())
		Expect(out).To(ContainSubstring("Stash pop failed"))

		// HEAD advanced — the rebase itself succeeded.
		headAfter := git(dir, "rev-parse", "HEAD")
		Expect(headAfter).NotTo(Equal(headBefore))

		reviewHead := git(dir, "rev-parse", reviewBranch)
		Expect(lineGit.IsAncestor(dir, reviewHead, "HEAD")).To(BeTrue())

		// Stash still exists for manual recovery.
		stashList := git(dir, "stash", "list")
		Expect(stashList).To(ContainSubstring("line-rebase: stashing WIP"))

		// No lingering rebase state.
		noRebaseInProgress(dir)
	})

	It("--leave-conflicts leaves mid-rebase state with conflict markers [REB-4]", func() {
		setupStationBranches(dir)

		// Station modifies conflict.txt on the terminal branch.
		git(dir, "checkout", lineGit.StationBranchName("cleanup"))
		writeFile(dir, "conflict.txt", "station version\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "[skip line] add conflict.txt")
		git(dir, "checkout", "master")

		// Master also adds conflict.txt with different content.
		writeFile(dir, "conflict.txt", "master version\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add conflict.txt on master")

		out, err := line(dir, "rebase", "--leave-conflicts")
		Expect(err).To(HaveOccurred())

		// Git should be in mid-rebase state.
		Expect(filepath.Join(dir, ".git", "rebase-merge")).To(BeADirectory())

		// Output lists conflicted files.
		Expect(out).To(ContainSubstring("conflict.txt"))

		// Output includes resolution instructions.
		Expect(out).To(ContainSubstring("git add"))
		Expect(out).To(ContainSubstring("git rebase --continue"))
	})

	It("--leave-conflicts with dirty tree mentions stash and stash entry exists [REB-4]", func() {
		setupStationBranches(dir)

		// Create conflict on terminal branch.
		git(dir, "checkout", lineGit.StationBranchName("cleanup"))
		writeFile(dir, "conflict.txt", "station version\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "[skip line] add conflict.txt")
		git(dir, "checkout", "master")

		// Master also adds conflict.txt.
		writeFile(dir, "conflict.txt", "master version\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add conflict.txt on master")

		// Dirty working tree.
		writeFile(dir, "code.go", "precious WIP\n")

		out, err := line(dir, "rebase", "--leave-conflicts")
		Expect(err).To(HaveOccurred())

		// Output mentions stash.
		Expect(out).To(ContainSubstring("git stash pop"))

		// Stash entry exists (was not popped).
		stashList := git(dir, "stash", "list")
		Expect(stashList).To(ContainSubstring("line-rebase: stashing WIP"))
	})

	It("--leave-conflicts with no conflict works normally [REB-4]", func() {
		setupStationBranches(dir)

		out := lineOK(dir, "rebase", "--leave-conflicts")

		// Normal success — flag is a no-op when there's no conflict.
		Expect(out).To(ContainSubstring("Rebased onto line/stn/cleanup"))
		Expect(out).To(ContainSubstring("review.txt"))
		Expect(out).To(ContainSubstring("cleanup.txt"))

		// No lingering rebase state.
		noRebaseInProgress(dir)
	})

	It("errors when rebase already in progress [REB-4]", func() {
		setupStationBranches(dir)

		// Simulate a rebase in progress by creating the marker directory.
		Expect(os.MkdirAll(filepath.Join(dir, ".git", "rebase-merge"), 0o755)).To(Succeed())

		out, err := line(dir, "rebase", "--leave-conflicts")
		Expect(err).To(HaveOccurred())
		Expect(out).To(ContainSubstring("rebase already in progress"))
	})

	It("refuses to run when not on the watched branch [REB-1]", func() {
		setupStationBranches(dir)

		git(dir, "checkout", "-b", "feature-branch")

		out, err := line(dir, "rebase")
		Expect(err).To(HaveOccurred())
		Expect(out).To(ContainSubstring("must be on master"))
		Expect(out).To(ContainSubstring("currently on feature-branch"))
	})
})
