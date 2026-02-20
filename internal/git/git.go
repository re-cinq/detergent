package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/re-cinq/assembly-line/internal/fileutil"
)

// Repo wraps git operations for a repository.
type Repo struct {
	Dir string
}

// NewRepo creates a Repo for the given directory.
func NewRepo(dir string) *Repo {
	return &Repo{Dir: dir}
}

// run executes a git command in the repo directory.
func (r *Repo) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// HeadCommit returns the commit hash at HEAD for a given branch.
func (r *Repo) HeadCommit(branch string) (string, error) {
	return r.run("rev-parse", branch)
}

// BranchExists checks if a branch exists.
func (r *Repo) BranchExists(branch string) bool {
	_, err := r.run("rev-parse", "--verify", branch)
	return err == nil
}

// CreateBranch creates a new branch from a starting point.
func (r *Repo) CreateBranch(name, from string) error {
	_, err := r.run("branch", name, from)
	return err
}

// CreateWorktree creates a git worktree for a branch.
func (r *Repo) CreateWorktree(path, branch string) error {
	_, err := r.run("worktree", "add", path, branch)
	return err
}

// CommitsBetween returns commit hashes between two refs (exclusive of from, inclusive of to).
// If from is empty, returns all commits up to `to`.
func (r *Repo) CommitsBetween(from, to string) ([]string, error) {
	var rangeSpec string
	if from == "" {
		rangeSpec = to
	} else {
		rangeSpec = from + ".." + to
	}
	out, err := r.run("rev-list", rangeSpec)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// CommitMessage returns the full commit message for a given hash.
func (r *Repo) CommitMessage(hash string) (string, error) {
	return r.run("log", "-1", "--format=%B", hash)
}

// DiffForCommit returns the diff for a single commit.
func (r *Repo) DiffForCommit(hash string) (string, error) {
	return r.run("diff", hash+"~1", hash)
}

// AddNote adds a git note to a commit under the "line" namespace.
func (r *Repo) AddNote(commit, message string) error {
	_, err := r.run("notes", "--ref=line", "add", "-f", "-m", message, commit)
	return err
}

// EnsureIdentity sets user.name and user.email in the repo's local config
// if they are not already resolvable (e.g. via global config or environment).
// This prevents "Author identity unknown" errors in CI environments.
func (r *Repo) EnsureIdentity() {
	if _, err := r.run("config", "user.name"); err != nil {
		_, _ = r.run("config", "user.name", "line")
	}
	if _, err := r.run("config", "user.email"); err != nil {
		_, _ = r.run("config", "user.email", "line@localhost")
	}
}

// WorktreePath returns the expected worktree path for a concern.
func WorktreePath(repoDir, branchPrefix, concernName string) string {
	return fileutil.LineSubdir(repoDir, filepath.Join("worktrees", branchPrefix+concernName))
}

// FilesChangedInCommit returns the list of file paths changed in a single commit.
// Uses diff-tree which works correctly for root commits (no parent).
func (r *Repo) FilesChangedInCommit(hash string) ([]string, error) {
	out, err := r.run("diff-tree", "--no-commit-id", "-r", "--name-only", hash)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// HasChanges checks if there are any uncommitted changes in the worktree.
func (r *Repo) HasChanges() (bool, error) {
	out, err := r.run("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// StageAll stages all changes (including untracked files) in the worktree.
func (r *Repo) StageAll() error {
	_, err := r.run("add", "-A")
	return err
}

// Commit creates a commit with the given message.
func (r *Repo) Commit(message string) error {
	_, err := r.run("commit", "-m", message)
	return err
}

// ResetSoft performs a soft reset to the given ref, preserving file changes.
func (r *Repo) ResetSoft(ref string) error {
	_, err := r.run("reset", "--soft", ref)
	return err
}

// Rebase rebases the current branch onto targetBranch.
// If conflicts occur, aborts the rebase and hard resets to targetBranch.
func (r *Repo) Rebase(targetBranch string) error {
	// Abort any stale in-progress rebase from a previous interrupted run.
	abortCmd := exec.Command("git", "rebase", "--abort")
	abortCmd.Dir = r.Dir
	_, _ = abortCmd.CombinedOutput() // ignore error — fails if no rebase in progress

	_, err := r.run("rebase", targetBranch)
	if err != nil {
		// Rebase conflict — abort and reset to target branch.
		// Concern branches are auto-generated; stale commits that
		// conflict with upstream should be discarded so the agent
		// can regenerate from a clean base.
		abort := exec.Command("git", "rebase", "--abort")
		abort.Dir = r.Dir
		_, _ = abort.CombinedOutput()

		_, resetErr := r.run("reset", "--hard", targetBranch)
		if resetErr != nil {
			return fmt.Errorf("git rebase %s failed and reset also failed: %w", targetBranch, resetErr)
		}
		// Reset succeeded — branch now matches target, agent will redo work
	}
	return nil
}
