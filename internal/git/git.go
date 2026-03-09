package git

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitEnvKeys lists git environment variable names that must be stripped from
// child processes. When line is invoked from a git hook (e.g. post-commit),
// git sets GIT_DIR and friends relative to the repo. If these leak into
// worktree subprocesses, GIT_DIR=.git resolves to a *file* (not a directory)
// inside the worktree, causing "index file open failed: Not a directory".
var gitEnvKeys = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_COMMON_DIR",
}

// CleanEnv returns a copy of environ with any variables matching the given key
// names removed. Key names should not include the "=" separator.
func CleanEnv(environ []string, keys ...string) []string {
	result := make([]string, 0, len(environ))
	for _, e := range environ {
		skip := false
		for _, key := range keys {
			if strings.HasPrefix(e, key+"=") {
				skip = true
				break
			}
		}
		if !skip {
			result = append(result, e)
		}
	}
	return result
}

// Run executes a git command in the given directory.
func Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(CleanEnv(os.Environ(), gitEnvKeys...), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CurrentBranch returns the current branch name.
func CurrentBranch(dir string) (string, error) {
	return Run(dir, "rev-parse", "--abbrev-ref", "HEAD")
}

// BranchExists checks if a branch exists.
func BranchExists(dir, branch string) bool {
	_, err := Run(dir, "rev-parse", "--verify", branch)
	return err == nil
}

// CreateBranch creates a new branch from a starting point.
func CreateBranch(dir, branch, startPoint string) error {
	_, err := Run(dir, "branch", branch, startPoint)
	return err
}

// Rebase rebases the current branch onto the given ref.
func Rebase(dir, onto string) error {
	_, err := Run(dir, "rebase", onto)
	return err
}

// RebaseAbort aborts an in-progress rebase.
func RebaseAbort(dir string) error {
	_, err := Run(dir, "rebase", "--abort")
	return err
}

// CommitAll stages all changes and commits with the given message.
// It excludes the .line/ directory which contains runtime state.
func CommitAll(dir, message string) error {
	if _, err := Run(dir, "add", "-A"); err != nil {
		return err
	}
	// Unstage .line/ - it's runtime state, not project code
	_, _ = Run(dir, "reset", "--", ".line/")
	// Check if there's anything to commit
	status, err := Run(dir, "status", "--porcelain")
	if err != nil {
		return err
	}
	if status == "" {
		return nil // Nothing to commit
	}
	_, err = Run(dir, "commit", "-m", message)
	return err
}

// HeadShortRef returns the short ref of HEAD.
func HeadShortRef(dir string) (string, error) {
	return Run(dir, "rev-parse", "--short", "HEAD")
}

// IsDirty returns true if the working tree has changes.
func IsDirty(dir string) (bool, error) {
	out, err := Run(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// ConflictedFiles returns the list of files with unresolved merge conflicts.
func ConflictedFiles(dir string) ([]string, error) {
	out, err := Run(dir, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// DiffFiles returns the list of changed file paths between two refs.
func DiffFiles(dir, from, to string) ([]string, error) {
	out, err := Run(dir, "diff", "--name-only", from, to)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// LastCommitMessage returns the message of the most recent commit.
func LastCommitMessage(dir string) (string, error) {
	return Run(dir, "log", "-1", "--format=%s")
}

// StationBranchName returns the branch name for a station.
func StationBranchName(name string) string {
	return "line/stn/" + name
}

// ResetHard resets the current branch to the given ref.
func ResetHard(dir, ref string) error {
	_, err := Run(dir, "reset", "--hard", ref)
	return err
}

// RevDistance returns how many commits ref2 is ahead of and behind ref1.
// ahead = commits in ref2 not in ref1, behind = commits in ref1 not in ref2.
func RevDistance(dir, ref1, ref2 string) (ahead, behind int, err error) {
	out, err := Run(dir, "rev-list", "--left-right", "--count", ref1+"..."+ref2)
	if err != nil {
		return 0, 0, err
	}
	_, err = fmt.Sscanf(out, "%d\t%d", &behind, &ahead)
	return
}

// IsAncestor returns true if ancestor's HEAD is reachable from descendant.
func IsAncestor(dir, ancestor, descendant string) bool {
	_, err := Run(dir, "merge-base", "--is-ancestor", ancestor, descendant)
	return err == nil
}

// HasCommitsBetween returns true if there are commits between from and to.
func HasCommitsBetween(dir, from, to string) (bool, error) {
	out, err := Run(dir, "rev-list", "--count", from+".."+to)
	if err != nil {
		return false, err
	}
	return out != "0", nil
}

// OnlySkipCommitsBetween returns true if from..to contains at least one commit
// and every commit message contains a skip marker.
func OnlySkipCommitsBetween(dir, from, to string, skipMarkers []string) bool {
	out, err := Run(dir, "log", "--format=%s", from+".."+to)
	if err != nil || out == "" {
		return false
	}
	for _, subject := range strings.Split(out, "\n") {
		hasMarker := false
		for _, marker := range skipMarkers {
			if strings.Contains(subject, marker) {
				hasMarker = true
				break
			}
		}
		if !hasMarker {
			return false
		}
	}
	return true
}

// WorktreeBaseDir returns a deterministic temp directory path for worktrees
// belonging to the given repo: /tmp/line-<8-char-sha256-of-abs-repo-path>/
func WorktreeBaseDir(repoDir string) (string, error) {
	abs, err := filepath.Abs(repoDir)
	if err != nil {
		return "", fmt.Errorf("resolving repo path: %w", err)
	}
	// Resolve symlinks to get a canonical path (e.g. /var -> /private/var on macOS)
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolving symlinks: %w", err)
	}
	h := sha256.Sum256([]byte(abs))
	tag := hex.EncodeToString(h[:])[:8]
	return filepath.Join(os.TempDir(), "line-"+tag), nil
}

// AddWorktree creates a git worktree at worktreePath for the given branch.
func AddWorktree(repoDir, worktreePath, branch string) error {
	_, err := Run(repoDir, "worktree", "add", worktreePath, branch)
	return err
}

// RemoveWorktree force-removes a git worktree.
func RemoveWorktree(repoDir, worktreePath string) error {
	_, err := Run(repoDir, "worktree", "remove", "--force", worktreePath)
	return err
}

// StashPush stashes uncommitted changes with a message.
func StashPush(dir, message string) error {
	_, err := Run(dir, "stash", "push", "-m", message)
	return err
}

// StashPop pops the most recent stash entry.
func StashPop(dir string) error {
	_, err := Run(dir, "stash", "pop")
	return err
}

// DeleteBranch force-deletes a local branch.
func DeleteBranch(dir, name string) error {
	_, err := Run(dir, "branch", "-D", name)
	return err
}

// PruneWorktrees prunes stale worktree bookkeeping entries.
func PruneWorktrees(repoDir string) error {
	_, err := Run(repoDir, "worktree", "prune")
	return err
}
