package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fission-ai/detergent/internal/config"
	gitops "github.com/fission-ai/detergent/internal/git"
)

// RunOnce processes each concern once and returns.
func RunOnce(cfg *config.Config, repoDir string) error {
	repo := gitops.NewRepo(repoDir)

	// Process concerns in dependency order (roots first)
	order := topologicalOrder(cfg)
	for _, concern := range order {
		if err := processConcern(cfg, repo, repoDir, concern); err != nil {
			return fmt.Errorf("concern %s: %w", concern.Name, err)
		}
	}
	return nil
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

	// Check for changes and commit
	if err := commitChanges(wtPath, concern, head); err != nil {
		return fmt.Errorf("committing changes: %w", err)
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

func commitChanges(worktreeDir string, concern config.Concern, triggeredBy string) error {
	// Check if there are changes
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = worktreeDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	status := strings.TrimSpace(string(out))
	if status == "" {
		return nil // no changes
	}

	// Stage all changes
	stageCmd := exec.Command("git", "add", "-A")
	stageCmd.Dir = worktreeDir
	if _, err := stageCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}

	// Build commit message
	msg := fmt.Sprintf("[%s] Agent changes\n\nTriggered-By: %s",
		strings.ToUpper(concern.Name), triggeredBy)

	commitCmd := exec.Command("git", "commit", "-m", msg)
	commitCmd.Dir = worktreeDir
	if commitOut, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("committing: %s: %w", string(commitOut), err)
	}

	return nil
}

// topologicalOrder returns concerns sorted so that dependencies come before dependents.
func topologicalOrder(cfg *config.Config) []config.Concern {
	nameSet := make(map[string]bool)
	for _, c := range cfg.Concerns {
		nameSet[c.Name] = true
	}

	byName := make(map[string]config.Concern)
	for _, c := range cfg.Concerns {
		byName[c.Name] = c
	}

	visited := make(map[string]bool)
	var result []config.Concern

	var visit func(name string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true

		c := byName[name]
		// Visit dependency first (if it's another concern)
		if nameSet[c.Watches] {
			visit(c.Watches)
		}

		result = append(result, c)
	}

	for _, c := range cfg.Concerns {
		visit(c.Name)
	}

	return result
}
