package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fission-ai/detergent/internal/config"
	gitops "github.com/fission-ai/detergent/internal/git"
)

// RunOnce processes each concern once and returns.
// Independent concerns at the same level run in parallel.
// Individual concern failures are logged but don't stop other concerns.
func RunOnce(cfg *config.Config, repoDir string) error {
	repo := gitops.NewRepo(repoDir)

	levels := topologicalLevels(cfg)
	failed := &failedSet{m: make(map[string]bool)}

	for _, level := range levels {
		if len(level) == 1 {
			// Single concern: run directly (no goroutine overhead)
			c := level[0]
			if failed.has(c.Watches) {
				fmt.Fprintf(os.Stderr, "skipping %s: upstream concern failed\n", c.Name)
				continue
			}
			if err := processConcern(cfg, repo, repoDir, c); err != nil {
				fmt.Fprintf(os.Stderr, "concern %s failed: %s\n", c.Name, err)
				failed.set(c.Name)
			}
		} else {
			// Multiple independent concerns: run in parallel
			var wg sync.WaitGroup
			for _, c := range level {
				if failed.has(c.Watches) {
					fmt.Fprintf(os.Stderr, "skipping %s: upstream concern failed\n", c.Name)
					continue
				}
				wg.Add(1)
				go func(concern config.Concern) {
					defer wg.Done()
					if err := processConcern(cfg, repo, repoDir, concern); err != nil {
						fmt.Fprintf(os.Stderr, "concern %s failed: %s\n", concern.Name, err)
						failed.set(concern.Name)
					}
				}(c)
			}
			wg.Wait()
		}
	}
	return nil
}

type failedSet struct {
	mu sync.Mutex
	m  map[string]bool
}

func (f *failedSet) set(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.m[name] = true
}

func (f *failedSet) has(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.m[name]
}

func processConcern(cfg *config.Config, repo *gitops.Repo, repoDir string, concern config.Concern) error {
	watchedBranch := resolveWatchedBranch(cfg, concern)

	// Get current HEAD of watched branch
	head, err := repo.HeadCommit(watchedBranch)
	if err != nil {
		return fmt.Errorf("getting HEAD of %s: %w", watchedBranch, err)
	}

	// Check last-seen
	lastSeen, err := LastSeen(repoDir, concern.Name)
	if err != nil {
		return err
	}
	if lastSeen == head {
		return nil // nothing new
	}

	outputBranch := cfg.Settings.BranchPrefix + concern.Name

	// Ensure output branch exists
	if !repo.BranchExists(outputBranch) {
		if err := repo.CreateBranch(outputBranch, watchedBranch); err != nil {
			return fmt.Errorf("creating output branch %s: %w", outputBranch, err)
		}
	}

	// Ensure worktree exists
	wtPath := gitops.WorktreePath(repoDir, cfg.Settings.BranchPrefix, concern.Name)
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(wtPath), 0755); err != nil {
			return err
		}
		if err := repo.CreateWorktree(wtPath, outputBranch); err != nil {
			return fmt.Errorf("creating worktree: %w", err)
		}
	}

	// Assemble context
	context, err := assembleContext(repo, concern, lastSeen, head)
	if err != nil {
		return fmt.Errorf("assembling context: %w", err)
	}

	// Invoke agent in worktree
	if err := invokeAgent(cfg, wtPath, context); err != nil {
		return fmt.Errorf("invoking agent: %w", err)
	}

	// Check for changes and commit (or fast-forward if no changes)
	changed, err := commitChanges(wtPath, concern, head)
	if err != nil {
		return fmt.Errorf("committing changes: %w", err)
	}

	if !changed {
		// No changes: fast-forward the output branch via merge in worktree
		if err := fastForwardWorktree(wtPath, watchedBranch); err != nil {
			return fmt.Errorf("fast-forwarding %s: %w", outputBranch, err)
		}
		// Add git note to each processed commit
		commits, _ := repo.CommitsBetween(lastSeen, head)
		noteMsg := fmt.Sprintf("[%s] Reviewed, no changes needed", strings.ToUpper(concern.Name))
		for _, hash := range commits {
			repo.AddNote(hash, noteMsg)
		}
	}

	// Update last-seen
	return SetLastSeen(repoDir, concern.Name, head)
}

func resolveWatchedBranch(cfg *config.Config, concern config.Concern) string {
	// If the concern watches another concern, resolve to its output branch
	for _, c := range cfg.Concerns {
		if c.Name == concern.Watches {
			return cfg.Settings.BranchPrefix + c.Name
		}
	}
	// Otherwise it's an external branch name
	return concern.Watches
}

func assembleContext(repo *gitops.Repo, concern config.Concern, lastSeen, head string) (string, error) {
	commits, err := repo.CommitsBetween(lastSeen, head)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("# Concern: " + concern.Name + "\n\n")
	sb.WriteString("## Prompt\n\n")
	sb.WriteString(concern.Prompt + "\n\n")
	sb.WriteString("## New commits to review\n\n")

	for _, hash := range commits {
		msg, err := repo.CommitMessage(hash)
		if err != nil {
			return "", err
		}
		sb.WriteString("### Commit " + hash[:8] + "\n")
		sb.WriteString("Message: " + msg + "\n\n")

		// Try to get diff (may fail for initial commit)
		diff, err := repo.DiffForCommit(hash)
		if err == nil && diff != "" {
			sb.WriteString("```diff\n" + diff + "\n```\n\n")
		}
	}

	return sb.String(), nil
}

func invokeAgent(cfg *config.Config, worktreeDir, context string) error {
	// Write context to a temp file
	contextFile := filepath.Join(worktreeDir, ".detergent-context")
	if err := os.WriteFile(contextFile, []byte(context), 0644); err != nil {
		return err
	}
	defer os.Remove(contextFile)

	args := append(cfg.Agent.Args, contextFile)
	cmd := exec.Command(cfg.Agent.Command, args...)
	cmd.Dir = worktreeDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func commitChanges(worktreeDir string, concern config.Concern, triggeredBy string) (bool, error) {
	// Check if there are changes
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = worktreeDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, err
	}

	status := strings.TrimSpace(string(out))
	if status == "" {
		return false, nil // no changes
	}

	// Stage all changes
	stageCmd := exec.Command("git", "add", "-A")
	stageCmd.Dir = worktreeDir
	if _, err := stageCmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("staging changes: %w", err)
	}

	// Build commit message
	msg := fmt.Sprintf("[%s] Agent changes\n\nTriggered-By: %s",
		strings.ToUpper(concern.Name), triggeredBy)

	commitCmd := exec.Command("git", "commit", "-m", msg)
	commitCmd.Dir = worktreeDir
	if commitOut, err := commitCmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("committing: %s: %w", string(commitOut), err)
	}

	return true, nil
}

func fastForwardWorktree(worktreeDir, targetBranch string) error {
	cmd := exec.Command("git", "merge", "--ff-only", targetBranch)
	cmd.Dir = worktreeDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git merge --ff-only %s: %s: %w", targetBranch, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// topologicalLevels groups concerns into levels for parallel execution.
// Level 0 = roots (watch external branches), Level 1 = depends only on level 0, etc.
func topologicalLevels(cfg *config.Config) [][]config.Concern {
	nameSet := make(map[string]bool)
	for _, c := range cfg.Concerns {
		nameSet[c.Name] = true
	}

	byName := make(map[string]config.Concern)
	for _, c := range cfg.Concerns {
		byName[c.Name] = c
	}

	// Compute level for each concern
	levels := make(map[string]int)
	var computeLevel func(name string) int
	computeLevel = func(name string) int {
		if l, ok := levels[name]; ok {
			return l
		}
		c := byName[name]
		if !nameSet[c.Watches] {
			levels[name] = 0
			return 0
		}
		l := computeLevel(c.Watches) + 1
		levels[name] = l
		return l
	}

	maxLevel := 0
	for _, c := range cfg.Concerns {
		l := computeLevel(c.Name)
		if l > maxLevel {
			maxLevel = l
		}
	}

	// Group by level
	result := make([][]config.Concern, maxLevel+1)
	for _, c := range cfg.Concerns {
		l := levels[c.Name]
		result[l] = append(result[l], c)
	}

	return result
}
