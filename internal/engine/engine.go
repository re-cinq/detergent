package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/re-cinq/detergent/internal/config"
	gitops "github.com/re-cinq/detergent/internal/git"
)

// LogManager manages per-concern log files for agent output.
type LogManager struct {
	mu    sync.Mutex
	files map[string]*os.File
}

// NewLogManager creates a new LogManager instance.
func NewLogManager() *LogManager {
	return &LogManager{
		files: make(map[string]*os.File),
	}
}

// getLogFile returns the log file for a concern, creating it if necessary.
// Log files are stored in the system temp directory with the pattern detergent-<concern>.log.
func (lm *LogManager) getLogFile(concernName string) (*os.File, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if f, ok := lm.files[concernName]; ok {
		return f, nil
	}

	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("detergent-%s.log", concernName))
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening log file %s: %w", logPath, err)
	}

	lm.files[concernName] = f
	return f, nil
}

// LogPath returns the log file path pattern for display purposes.
func LogPath() string {
	return filepath.Join(os.TempDir(), "detergent-<concern>.log")
}

// LogPathFor returns the log file path for a specific concern.
func LogPathFor(concernName string) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("detergent-%s.log", concernName))
}

// Close closes all open log files.
func (lm *LogManager) Close() error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	var firstErr error
	for name, f := range lm.files {
		if err := f.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("closing log file for %s: %w", name, err)
		}
	}
	lm.files = make(map[string]*os.File)
	return firstErr
}

// RunOnce processes each concern once and returns.
// Independent concerns at the same level run in parallel.
// Individual concern failures are logged but don't stop other concerns.
// Creates a temporary LogManager that is closed after processing.
func RunOnce(cfg *config.Config, repoDir string) error {
	logMgr := NewLogManager()
	defer logMgr.Close()
	return RunOnceWithLogs(cfg, repoDir, logMgr)
}

// RunOnceWithLogs processes each concern once using the provided LogManager.
// The LogManager is not closed; the caller is responsible for closing it.
func RunOnceWithLogs(cfg *config.Config, repoDir string, logMgr *LogManager) error {
	repo := gitops.NewRepo(repoDir)
	repo.EnsureIdentity()

	levels := topologicalLevels(cfg)
	failed := &failedSet{m: make(map[string]bool)}

	for _, level := range levels {
		if len(level) == 1 {
			// Single concern: run directly (no goroutine overhead)
			c := level[0]
			if failed.has(c.Watches) {
				fmt.Fprintf(os.Stderr, "skipping %s: upstream concern failed\n", c.Name)
				_ = WriteStatus(repoDir, c.Name, &ConcernStatus{
					State: "skipped",
					Error: "upstream concern failed",
					PID:   os.Getpid(),
				})
				continue
			}
			if err := processConcern(cfg, repo, repoDir, c, logMgr); err != nil {
				fmt.Fprintf(os.Stderr, "concern %s failed: %s\n", c.Name, err)
				failed.set(c.Name)
			}
		} else {
			// Multiple independent concerns: run in parallel
			var wg sync.WaitGroup
			for _, c := range level {
				if failed.has(c.Watches) {
					fmt.Fprintf(os.Stderr, "skipping %s: upstream concern failed\n", c.Name)
					_ = WriteStatus(repoDir, c.Name, &ConcernStatus{
						State: "skipped",
						Error: "upstream concern failed",
						PID:   os.Getpid(),
					})
					continue
				}
				wg.Add(1)
				go func(concern config.Concern) {
					defer wg.Done()
					if err := processConcern(cfg, repo, repoDir, concern, logMgr); err != nil {
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

func processConcern(cfg *config.Config, repo *gitops.Repo, repoDir string, concern config.Concern, logMgr *LogManager) error {
	pid := os.Getpid()
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
		// Nothing new — preserve last_result from previous run
		prevStatus, _ := ReadStatus(repoDir, concern.Name)
		lastResult := ""
		if prevStatus != nil {
			lastResult = prevStatus.LastResult
		}
		_ = WriteStatus(repoDir, concern.Name, &ConcernStatus{
			State:      "idle",
			LastSeen:   lastSeen,
			LastResult: lastResult,
			PID:        pid,
		})
		return nil // nothing new
	}

	// Check if all new commits have skip markers
	if allCommitsSkipped(repo, lastSeen, head) {
		// Advance last-seen so we don't re-check these commits
		if err := SetLastSeen(repoDir, concern.Name, head); err != nil {
			return fmt.Errorf("updating last-seen after skip: %w", err)
		}
		prevStatus, _ := ReadStatus(repoDir, concern.Name)
		lastResult := ""
		if prevStatus != nil {
			lastResult = prevStatus.LastResult
		}
		_ = WriteStatus(repoDir, concern.Name, &ConcernStatus{
			State:      "idle",
			LastSeen:   head,
			LastResult: lastResult,
			PID:        pid,
		})
		return nil
	}

	// Write change-detected status
	startedAt := nowRFC3339()
	_ = WriteStatus(repoDir, concern.Name, &ConcernStatus{
		State:       "change_detected",
		StartedAt:   startedAt,
		HeadAtStart: head,
		LastSeen:    lastSeen,
		PID:         pid,
	})

	outputBranch := cfg.Settings.BranchPrefix + concern.Name

	// Ensure output branch exists
	if !repo.BranchExists(outputBranch) {
		if err := repo.CreateBranch(outputBranch, watchedBranch); err != nil {
			return processConcernFailed(repoDir, concern.Name, startedAt, head, lastSeen, pid, err,
				fmt.Errorf("creating output branch %s: %w", outputBranch, err))
		}
	}

	// Ensure worktree exists
	wtPath := gitops.WorktreePath(repoDir, cfg.Settings.BranchPrefix, concern.Name)
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(wtPath), 0755); err != nil {
			return processConcernFailed(repoDir, concern.Name, startedAt, head, lastSeen, pid, err,
				fmt.Errorf("creating worktree directory: %w", err))
		}
		if err := repo.CreateWorktree(wtPath, outputBranch); err != nil {
			return processConcernFailed(repoDir, concern.Name, startedAt, head, lastSeen, pid, err,
				fmt.Errorf("creating worktree: %w", err))
		}
	}

	// Rebase output branch onto watched branch so prior concern
	// commits sit on top of the latest upstream state.
	if err := rebaseWorktree(wtPath, watchedBranch); err != nil {
		return processConcernFailed(repoDir, concern.Name, startedAt, head, lastSeen, pid, err,
			fmt.Errorf("rebasing %s onto %s: %w", outputBranch, watchedBranch, err))
	}

	// Assemble context
	context, err := assembleContext(repo, concern, lastSeen, head)
	if err != nil {
		return processConcernFailed(repoDir, concern.Name, startedAt, head, lastSeen, pid, err,
			fmt.Errorf("assembling context: %w", err))
	}

	// Get log file for this concern
	logFile, err := logMgr.getLogFile(concern.Name)
	if err != nil {
		return processConcernFailed(repoDir, concern.Name, startedAt, head, lastSeen, pid, err,
			fmt.Errorf("getting log file: %w", err))
	}

	// Write commit context header to log file
	header := fmt.Sprintf("--- Processing %s at %s ---\n", head, time.Now().UTC().Format(time.RFC3339))
	if _, err := logFile.WriteString(header); err != nil {
		return processConcernFailed(repoDir, concern.Name, startedAt, head, lastSeen, pid, err,
			fmt.Errorf("writing log header: %w", err))
	}

	// Write agent-started status
	_ = WriteStatus(repoDir, concern.Name, &ConcernStatus{
		State:       "agent_running",
		StartedAt:   startedAt,
		HeadAtStart: head,
		LastSeen:    lastSeen,
		PID:         pid,
	})

	// Invoke agent in worktree
	if err := invokeAgent(cfg, wtPath, context, logFile); err != nil {
		return processConcernFailed(repoDir, concern.Name, startedAt, head, lastSeen, pid, err,
			fmt.Errorf("invoking agent: %w", err))
	}

	// Write agent-succeeded status
	_ = WriteStatus(repoDir, concern.Name, &ConcernStatus{
		State:       "committing",
		StartedAt:   startedAt,
		HeadAtStart: head,
		LastSeen:    lastSeen,
		PID:         pid,
	})

	// Check for changes and commit
	changed, err := commitChanges(wtPath, concern, head)
	if err != nil {
		return processConcernFailed(repoDir, concern.Name, startedAt, head, lastSeen, pid, err,
			fmt.Errorf("committing changes: %w", err))
	}

	if !changed {
		// Branch already at or ahead of watched after rebase — just add notes
		commits, _ := repo.CommitsBetween(lastSeen, head)
		noteMsg := fmt.Sprintf("[%s] Reviewed, no changes needed", strings.ToUpper(concern.Name))
		for _, hash := range commits {
			_ = repo.AddNote(hash, noteMsg)
		}
	}

	// Update last-seen
	if err := SetLastSeen(repoDir, concern.Name, head); err != nil {
		return processConcernFailed(repoDir, concern.Name, startedAt, head, lastSeen, pid, err,
			fmt.Errorf("updating last-seen marker: %w", err))
	}

	// Write idle status with result
	result := "noop"
	if changed {
		result = "modified"
	}
	_ = WriteStatus(repoDir, concern.Name, &ConcernStatus{
		State:       "idle",
		LastResult:  result,
		StartedAt:   startedAt,
		CompletedAt: nowRFC3339(),
		LastSeen:    head,
		HeadAtStart: head,
		PID:         pid,
	})

	return nil
}

// processConcernFailed writes a failed status and returns the wrapped error.
func processConcernFailed(repoDir, concernName, startedAt, head, lastSeen string, pid int, origErr, wrappedErr error) error {
	_ = WriteStatus(repoDir, concernName, &ConcernStatus{
		State:       "failed",
		StartedAt:   startedAt,
		CompletedAt: nowRFC3339(),
		Error:       origErr.Error(),
		LastSeen:    lastSeen,
		HeadAtStart: head,
		PID:         pid,
	})
	return wrappedErr
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

func invokeAgent(cfg *config.Config, worktreeDir, context string, output io.Writer) error {
	// Write context to a file in the worktree (available to the agent)
	contextFile := filepath.Join(worktreeDir, ".detergent-context")
	if err := os.WriteFile(contextFile, []byte(context), 0644); err != nil {
		return err
	}
	defer os.Remove(contextFile)

	// Write permissions settings if configured
	if cfg.Permissions != nil {
		if err := writePermissions(worktreeDir, cfg.Permissions); err != nil {
			return fmt.Errorf("writing permissions: %w", err)
		}
	}

	// Pass context file path as last arg, and pipe context to stdin
	// so agents like `claude -p` that read from stdin work too
	args := append(cfg.Agent.Args, contextFile)
	cmd := exec.Command(cfg.Agent.Command, args...)
	cmd.Dir = worktreeDir

	// Allocate a PTY for stdout/stderr so the agent sees a terminal and uses
	// line buffering, enabling real-time log tailing via `status -f` / `logs -f`.
	// Stdin stays as a regular pipe so the agent gets a proper EOF.
	//
	// NOTE: this works (verified by integration tests with Python proving
	// real-time line-buffered output), but Claude Code batches its output
	// internally regardless of TTY detection, so logs appear in chunks
	// rather than streaming line-by-line.
	ptmx, pts, err := pty.Open()
	if err != nil {
		return fmt.Errorf("opening pty: %w", err)
	}
	defer ptmx.Close()

	cmd.Stdin = strings.NewReader(context)
	cmd.Stdout = pts
	cmd.Stderr = pts

	if err := cmd.Start(); err != nil {
		pts.Close()
		return fmt.Errorf("starting agent: %w", err)
	}
	pts.Close() // close slave in parent; child inherited it

	// Copy PTY output to the log file; ignore EIO at process exit
	if _, err := io.Copy(output, ptmx); err != nil {
		var pathErr *os.PathError
		if !(errors.As(err, &pathErr) && pathErr.Err == syscall.EIO) {
			return fmt.Errorf("reading agent output: %w", err)
		}
	}

	return cmd.Wait()
}

// writePermissions writes a .claude/settings.json file in the worktree
// with the configured permissions, so Claude Code agents get pre-approved tools.
func writePermissions(worktreeDir string, perms *config.Permissions) error {
	claudeDir := filepath.Join(worktreeDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return err
	}

	settings := map[string]interface{}{
		"permissions": perms,
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(claudeDir, "settings.json"), append(data, '\n'), 0644)
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

func rebaseWorktree(worktreeDir, targetBranch string) error {
	// Abort any stale in-progress rebase from a previous interrupted run.
	abortCmd := exec.Command("git", "rebase", "--abort")
	abortCmd.Dir = worktreeDir
	_, _ = abortCmd.CombinedOutput() // ignore error — fails if no rebase in progress

	cmd := exec.Command("git", "rebase", targetBranch)
	cmd.Dir = worktreeDir
	_, err := cmd.CombinedOutput()
	if err != nil {
		// Rebase conflict — abort and reset to target branch.
		// Concern branches are auto-generated; stale commits that
		// conflict with upstream should be discarded so the agent
		// can regenerate from a clean base.
		abort := exec.Command("git", "rebase", "--abort")
		abort.Dir = worktreeDir
		_, _ = abort.CombinedOutput()

		reset := exec.Command("git", "reset", "--hard", targetBranch)
		reset.Dir = worktreeDir
		if resetOut, resetErr := reset.CombinedOutput(); resetErr != nil {
			return fmt.Errorf("git rebase %s failed and reset also failed: %s: %w",
				targetBranch, strings.TrimSpace(string(resetOut)), resetErr)
		}
		// Reset succeeded — branch now matches target, agent will redo work
	}
	return nil
}

// allCommitsSkipped returns true if every commit between lastSeen and head
// contains a skip marker ([skip ci], [ci skip], [skip detergent], [detergent skip]).
// Returns false if there are no commits or if any commit lacks a skip marker.
func allCommitsSkipped(repo *gitops.Repo, lastSeen, head string) bool {
	commits, err := repo.CommitsBetween(lastSeen, head)
	if err != nil || len(commits) == 0 {
		return false
	}
	for _, hash := range commits {
		msg, err := repo.CommitMessage(hash)
		if err != nil {
			return false
		}
		if !hasSkipMarker(msg) {
			return false
		}
	}
	return true
}

// hasSkipMarker checks if a commit message contains a recognized skip marker.
func hasSkipMarker(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "[skip ci]") ||
		strings.Contains(lower, "[ci skip]") ||
		strings.Contains(lower, "[skip detergent]") ||
		strings.Contains(lower, "[detergent skip]")
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
