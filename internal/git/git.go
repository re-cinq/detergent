package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
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

// RemoveWorktree removes a git worktree.
func (r *Repo) RemoveWorktree(path string) error {
	_, err := r.run("worktree", "remove", "--force", path)
	return err
}

// WorktreeList returns the list of worktree paths.
func (r *Repo) WorktreeList() ([]string, error) {
	out, err := r.run("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			paths = append(paths, strings.TrimPrefix(line, "worktree "))
		}
	}
	return paths, nil
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

// FastForwardBranch updates a branch ref to point at a new commit (fast-forward).
func (r *Repo) FastForwardBranch(branch, targetCommit string) error {
	_, err := r.run("branch", "-f", branch, targetCommit)
	return err
}

// AddNote adds a git note to a commit under the "detergent" namespace.
func (r *Repo) AddNote(commit, message string) error {
	_, err := r.run("notes", "--ref=detergent", "add", "-f", "-m", message, commit)
	return err
}

// GetNote reads the git note for a commit under the "detergent" namespace.
func (r *Repo) GetNote(commit string) (string, error) {
	return r.run("notes", "--ref=detergent", "show", commit)
}

// WorktreePath returns the expected worktree path for a concern.
func WorktreePath(repoDir, branchPrefix, concernName string) string {
	return filepath.Join(repoDir, ".detergent", "worktrees", branchPrefix+concernName)
}
