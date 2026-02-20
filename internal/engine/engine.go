package engine

import (
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
	ignore "github.com/sabhiram/go-gitignore"

	"github.com/re-cinq/assembly-line/internal/config"
	"github.com/re-cinq/assembly-line/internal/fileutil"
	gitops "github.com/re-cinq/assembly-line/internal/git"
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
// Log files are stored in the system temp directory with the pattern line-<concern>.log.
func (lm *LogManager) getLogFile(concernName string) (*os.File, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if f, ok := lm.files[concernName]; ok {
		return f, nil
	}

	logPath := LogPathFor(concernName)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening log file %s: %w", logPath, err)
	}

	lm.files[concernName] = f
	return f, nil
}

// truncateLogFile truncates the log file for a concern to clear old logs from previous runs.
func (lm *LogManager) truncateLogFile(concernName string) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Close and remove the cached file handle if it exists
	if f, ok := lm.files[concernName]; ok {
		f.Close()
		delete(lm.files, concernName)
	}

	logPath := LogPathFor(concernName)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("truncating log file %s: %w", logPath, err)
	}

	lm.files[concernName] = f
	return nil
}

// LogPath returns the log file path pattern for display purposes.
func LogPath() string {
	return filepath.Join(os.TempDir(), "line-<concern>.log")
}

// LogPathFor returns the log file path for a specific concern.
func LogPathFor(concernName string) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("line-%s.log", concernName))
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

// shouldSkipConcern checks if a concern should be skipped due to upstream failures.
// If upstream failed, it writes a skip status and returns true.
func shouldSkipConcern(repoDir string, c config.Concern, failed *failedSet) bool {
	if failed.has(c.Watches) {
		skipUpstreamFailed(repoDir, c.Name, os.Getpid())
		return true
	}
	return false
}

// processConcernAndTrackFailure processes a concern and tracks failures in the failedSet.
func processConcernAndTrackFailure(cfg *config.Config, repo *gitops.Repo, repoDir string, c config.Concern, logMgr *LogManager, failed *failedSet) {
	if err := processConcern(cfg, repo, repoDir, c, logMgr); err != nil {
		fileutil.LogError("concern %s failed: %s", c.Name, err)
		failed.set(c.Name)
	}
}

// RunOnceWithLogs processes each concern once using the provided LogManager.
// The LogManager is not closed; the caller is responsible for closing it.
func RunOnceWithLogs(cfg *config.Config, repoDir string, logMgr *LogManager) error {
	// Clear any stale active statuses from a previous interrupted run.
	concernNames := make([]string, len(cfg.Concerns))
	for i, c := range cfg.Concerns {
		concernNames[i] = c.Name
	}
	ResetActiveStatuses(repoDir, concernNames)

	repo := gitops.NewRepo(repoDir)
	repo.EnsureIdentity()

	levels := topologicalLevels(cfg)
	failed := &failedSet{m: make(map[string]bool)}

	for _, level := range levels {
		if len(level) == 1 {
			// Single concern: run directly (no goroutine overhead)
			c := level[0]
			if !shouldSkipConcern(repoDir, c, failed) {
				processConcernAndTrackFailure(cfg, repo, repoDir, c, logMgr, failed)
			}
		} else {
			// Multiple independent concerns: run in parallel
			var wg sync.WaitGroup
			for _, c := range level {
				if shouldSkipConcern(repoDir, c, failed) {
					continue
				}
				wg.Add(1)
				go func(concern config.Concern) {
					defer wg.Done()
					processConcernAndTrackFailure(cfg, repo, repoDir, concern, logMgr, failed)
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

// concernContext holds the execution context for processing a single concern.
// It bundles frequently-used parameters to reduce function signatures.
type concernContext struct {
	repoDir     string
	concernName string
	startedAt   string
	head        string
	lastSeen    string
	pid         int
}

// fail writes a failed status and returns a wrapped error.
func (ctx *concernContext) fail(origErr error, wrappedErr error) error {
	return processConcernFailed(ctx.repoDir, ctx.concernName, ctx.startedAt,
		ctx.head, ctx.lastSeen, ctx.pid, origErr, wrappedErr)
}

func processConcern(cfg *config.Config, repo *gitops.Repo, repoDir string, concern config.Concern, logMgr *LogManager) error {
	pid := os.Getpid()
	watchedBranch := ResolveWatchedBranch(cfg, concern)

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
		writeIdleStatus(repoDir, concern.Name, lastSeen, pid)
		return nil // nothing new
	}

	// Check if all new commits have skip markers (or agent commits on external branches)
	skipAgentCommits := WatchesExternalBranch(cfg, concern)
	gi := loadIgnorePatterns(repoDir)
	if allCommitsSkipped(repo, lastSeen, head, skipAgentCommits, gi) {
		// Advance last-seen so we don't re-check these commits
		if err := SetLastSeen(repoDir, concern.Name, head); err != nil {
			return fmt.Errorf("updating last-seen after skip: %w", err)
		}
		writeIdleStatus(repoDir, concern.Name, head, pid)
		return nil
	}

	// Create execution context to reduce parameter passing
	ctx := &concernContext{
		repoDir:     repoDir,
		concernName: concern.Name,
		startedAt:   nowRFC3339(),
		head:        head,
		lastSeen:    lastSeen,
		pid:         pid,
	}

	// Write change-detected status
	writeChangeDetectedStatus(ctx.repoDir, ctx.concernName, ctx.startedAt, ctx.head, ctx.lastSeen, ctx.pid)

	outputBranch := cfg.Settings.BranchPrefix + concern.Name

	// Ensure output branch exists
	if !repo.BranchExists(outputBranch) {
		if err := repo.CreateBranch(outputBranch, watchedBranch); err != nil {
			return ctx.fail(err, fmt.Errorf("creating output branch %s: %w", outputBranch, err))
		}
	}

	// Ensure worktree exists
	wtPath := gitops.WorktreePath(repoDir, cfg.Settings.BranchPrefix, concern.Name)
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		if err := fileutil.EnsureDir(filepath.Dir(wtPath)); err != nil {
			return ctx.fail(err, fmt.Errorf("creating worktree directory: %w", err))
		}
		if err := repo.CreateWorktree(wtPath, outputBranch); err != nil {
			return ctx.fail(err, fmt.Errorf("creating worktree: %w", err))
		}
	}

	// Rebase output branch onto watched branch so prior concern
	// commits sit on top of the latest upstream state.
	if err := rebaseWorktree(wtPath, watchedBranch); err != nil {
		return ctx.fail(err, fmt.Errorf("rebasing %s onto %s: %w", outputBranch, watchedBranch, err))
	}

	// Assemble context
	context, err := assembleContext(repo, cfg, concern, lastSeen, head)
	if err != nil {
		return ctx.fail(err, fmt.Errorf("assembling context: %w", err))
	}

	// Truncate log file to clear old logs from previous runs
	if err := logMgr.truncateLogFile(concern.Name); err != nil {
		return ctx.fail(err, fmt.Errorf("truncating log file: %w", err))
	}

	// Get log file for this concern (after truncate, this returns the fresh file)
	logFile, err := logMgr.getLogFile(concern.Name)
	if err != nil {
		return ctx.fail(err, fmt.Errorf("getting log file: %w", err))
	}

	// Write commit context header to log file
	header := fmt.Sprintf("--- Processing %s at %s ---\n", head, time.Now().UTC().Format(time.RFC3339))
	if _, err := logFile.WriteString(header); err != nil {
		return ctx.fail(err, fmt.Errorf("writing log header: %w", err))
	}

	// Write agent-started status
	writeAgentRunningStatus(ctx.repoDir, ctx.concernName, ctx.startedAt, ctx.head, ctx.lastSeen, ctx.pid)

	// Snapshot worktree HEAD before agent runs so we can detect rogue commits
	wtRepo := gitops.NewRepo(wtPath)
	preAgentHead, err := wtRepo.HeadCommit("HEAD")
	if err != nil {
		return ctx.fail(err, fmt.Errorf("snapshotting worktree HEAD: %w", err))
	}

	// Invoke agent in worktree
	if err := invokeAgent(cfg, concern, wtPath, context, logFile); err != nil {
		return ctx.fail(err, fmt.Errorf("invoking agent: %w", err))
	}

	// Soft-reset any commits the agent made directly — we need the file
	// changes but will create a proper commit with Triggered-By trailers.
	postAgentHead, err := wtRepo.HeadCommit("HEAD")
	if err != nil {
		return ctx.fail(err, fmt.Errorf("checking worktree HEAD after agent: %w", err))
	}
	if postAgentHead != preAgentHead {
		fileutil.LogError("concern %s: agent made direct commits — soft-resetting to preserve changes", concern.Name)
		if err := wtRepo.ResetSoft(preAgentHead); err != nil {
			return ctx.fail(err, fmt.Errorf("soft-resetting agent commits: %w", err))
		}
	}

	// Write agent-succeeded status
	writeCommittingStatus(ctx.repoDir, ctx.concernName, ctx.startedAt, ctx.head, ctx.lastSeen, ctx.pid)

	// Check for changes and commit
	changed, err := commitChanges(wtPath, concern, head)
	if err != nil {
		return ctx.fail(err, fmt.Errorf("committing changes: %w", err))
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
		return ctx.fail(err, fmt.Errorf("updating last-seen marker: %w", err))
	}

	// Write idle status with result
	result := ResultNoop
	if changed {
		result = ResultModified
	}
	writeIdleWithResultStatus(ctx.repoDir, ctx.concernName, ctx.startedAt, nowRFC3339(), ctx.head, result, ctx.pid)

	return nil
}

// getLastResult retrieves the LastResult from the previous status, or "" if not found.
func getLastResult(repoDir, concernName string) string {
	prevStatus, _ := ReadStatus(repoDir, concernName)
	if prevStatus != nil {
		return prevStatus.LastResult
	}
	return ""
}

// statusUpdate holds optional fields for writing concern status.
// Zero values are omitted from the written status.
type statusUpdate struct {
	state       string
	startedAt   string
	completedAt string
	headAtStart string
	lastSeen    string
	lastResult  string
	errorMsg    string
	pid         int
}

// writeStatus writes a concern status with the given fields.
// This consolidates all status-writing into a single helper.
func writeStatus(repoDir, concernName string, u statusUpdate) {
	status := &ConcernStatus{
		State:       u.state,
		StartedAt:   u.startedAt,
		CompletedAt: u.completedAt,
		HeadAtStart: u.headAtStart,
		LastSeen:    u.lastSeen,
		LastResult:  u.lastResult,
		Error:       u.errorMsg,
		PID:         u.pid,
	}
	_ = WriteStatus(repoDir, concernName, status)
}

// writeChangeDetectedStatus writes a change-detected status.
func writeChangeDetectedStatus(repoDir, concernName, startedAt, head, lastSeen string, pid int) {
	writeStatus(repoDir, concernName, statusUpdate{
		state:       StateChangeDetected,
		startedAt:   startedAt,
		headAtStart: head,
		lastSeen:    lastSeen,
		pid:         pid,
	})
}

// writeAgentRunningStatus writes an agent-running status.
func writeAgentRunningStatus(repoDir, concernName, startedAt, head, lastSeen string, pid int) {
	writeStatus(repoDir, concernName, statusUpdate{
		state:       StateAgentRunning,
		startedAt:   startedAt,
		headAtStart: head,
		lastSeen:    lastSeen,
		pid:         pid,
	})
}

// writeCommittingStatus writes a committing status.
func writeCommittingStatus(repoDir, concernName, startedAt, head, lastSeen string, pid int) {
	writeStatus(repoDir, concernName, statusUpdate{
		state:       StateCommitting,
		startedAt:   startedAt,
		headAtStart: head,
		lastSeen:    lastSeen,
		pid:         pid,
	})
}

// writeIdleWithResultStatus writes an idle status with a specific result.
func writeIdleWithResultStatus(repoDir, concernName, startedAt, completedAt, head, result string, pid int) {
	writeStatus(repoDir, concernName, statusUpdate{
		state:       StateIdle,
		startedAt:   startedAt,
		completedAt: completedAt,
		headAtStart: head,
		lastSeen:    head,
		lastResult:  result,
		pid:         pid,
	})
}

// writeIdleStatus writes an idle status, preserving the previous LastResult.
func writeIdleStatus(repoDir, concernName, lastSeen string, pid int) {
	writeStatus(repoDir, concernName, statusUpdate{
		state:      StateIdle,
		lastSeen:   lastSeen,
		lastResult: getLastResult(repoDir, concernName),
		pid:        pid,
	})
}

// writeFailedStatus writes a failed status with completion timestamp and error.
func writeFailedStatus(repoDir, concernName, startedAt, completedAt, head, lastSeen, errorMsg string, pid int) {
	writeStatus(repoDir, concernName, statusUpdate{
		state:       StateFailed,
		startedAt:   startedAt,
		completedAt: completedAt,
		headAtStart: head,
		lastSeen:    lastSeen,
		errorMsg:    errorMsg,
		pid:         pid,
	})
}

// writeSkippedStatus writes a skipped status with the given error message.
func writeSkippedStatus(repoDir, concernName, errorMsg string, pid int) {
	writeStatus(repoDir, concernName, statusUpdate{
		state:    StateSkipped,
		errorMsg: errorMsg,
		pid:      pid,
	})
}

// skipUpstreamFailed logs and marks a concern as skipped due to upstream failure.
func skipUpstreamFailed(repoDir, concernName string, pid int) {
	fileutil.LogError("skipping %s: upstream concern failed", concernName)
	writeSkippedStatus(repoDir, concernName, "upstream concern failed", pid)
}

// processConcernFailed writes a failed status and returns the wrapped error.
func processConcernFailed(repoDir, concernName, startedAt, head, lastSeen string, pid int, origErr, wrappedErr error) error {
	writeFailedStatus(repoDir, concernName, startedAt, nowRFC3339(), head, lastSeen, origErr.Error(), pid)
	return wrappedErr
}

func ResolveWatchedBranch(cfg *config.Config, concern config.Concern) string {
	// If the concern watches another concern, resolve to its output branch
	for _, c := range cfg.Concerns {
		if c.Name == concern.Watches {
			return cfg.Settings.BranchPrefix + c.Name
		}
	}
	// Otherwise it's an external branch name
	return concern.Watches
}

// forEachCommitMessage iterates over commits and calls fn with each commit hash and message.
// Returns early with error if CommitMessage fails.
func forEachCommitMessage(repo *gitops.Repo, commits []string, fn func(hash, msg string) error) error {
	for _, hash := range commits {
		msg, err := repo.CommitMessage(hash)
		if err != nil {
			return err
		}
		if err := fn(hash, msg); err != nil {
			return err
		}
	}
	return nil
}

func assembleContext(repo *gitops.Repo, cfg *config.Config, concern config.Concern, lastSeen, head string) (string, error) {
	commits, err := repo.CommitsBetween(lastSeen, head)
	if err != nil {
		return "", err
	}

	// When watching an external branch, filter out agent commits from context.
	// After a rebase, these are our own output coming back — not new work.
	skipAgent := WatchesExternalBranch(cfg, concern)

	var sb strings.Builder
	sb.WriteString("You are running non-interactively. Do not ask questions or wait for confirmation.\nIf something is unclear, make your best judgement and proceed.\nDo not run git commit — your changes will be committed automatically.\n\n")
	sb.WriteString("# Concern: " + concern.Name + "\n\n")
	sb.WriteString("## Prompt\n\n")
	sb.WriteString(concern.Prompt + "\n\n")
	sb.WriteString("## New commits to review\n\n")

	err = forEachCommitMessage(repo, commits, func(hash, msg string) error {
		if skipAgent && isAgentCommit(msg) {
			return nil
		}
		sb.WriteString("### Commit " + hash[:8] + "\n")
		sb.WriteString("Message: " + msg + "\n\n")

		// Try to get diff (may fail for initial commit)
		diff, err := repo.DiffForCommit(hash)
		if err == nil && diff != "" {
			sb.WriteString("```diff\n" + diff + "\n```\n\n")
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	return sb.String(), nil
}

func invokeAgent(cfg *config.Config, concern config.Concern, worktreeDir, context string, output io.Writer) error {
	// Write context to a file in the worktree (available to the agent)
	contextFile := filepath.Join(worktreeDir, ".line-context")
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

	// Resolve command and args: per-concern overrides take precedence over global
	agentCommand := cfg.Agent.Command
	if concern.Command != "" {
		agentCommand = concern.Command
	}
	agentArgs := cfg.Agent.Args
	if concern.Args != nil {
		agentArgs = concern.Args
	}

	// Pass context file path as last arg, and pipe context to stdin
	// so agents like `claude -p` that read from stdin work too
	args := append(agentArgs, contextFile)
	cmd := exec.Command(agentCommand, args...)
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
	claudeDir := fileutil.ClaudeDir(worktreeDir)
	if err := fileutil.EnsureDir(claudeDir); err != nil {
		return err
	}

	settings := map[string]interface{}{
		"permissions": perms,
	}
	return fileutil.WriteJSON(fileutil.ClaudeSubpath(worktreeDir, "settings.json"), settings)
}

func commitChanges(worktreeDir string, concern config.Concern, triggeredBy string) (bool, error) {
	repo := gitops.NewRepo(worktreeDir)

	hasChanges, err := repo.HasChanges()
	if err != nil {
		return false, err
	}
	if !hasChanges {
		return false, nil // no changes
	}

	if err := repo.StageAll(); err != nil {
		return false, fmt.Errorf("staging changes: %w", err)
	}

	msg := fmt.Sprintf("[%s] Agent changes\n\nTriggered-By: %s",
		strings.ToUpper(concern.Name), triggeredBy)

	if err := repo.Commit(msg); err != nil {
		return false, fmt.Errorf("committing: %w", err)
	}

	return true, nil
}

func rebaseWorktree(worktreeDir, targetBranch string) error {
	repo := gitops.NewRepo(worktreeDir)
	return repo.Rebase(targetBranch)
}

// loadIgnorePatterns reads a .lineignore file from the repo root.
// Returns nil if the file does not exist.
func loadIgnorePatterns(repoDir string) *ignore.GitIgnore {
	path := filepath.Join(repoDir, ".lineignore")
	gi, err := ignore.CompileIgnoreFile(path)
	if err != nil {
		return nil
	}
	return gi
}

// filesMatchIgnorePatterns returns true if all files match the ignore patterns.
// Returns false if gi is nil, files is empty, or .lineignore itself is in the list.
func filesMatchIgnorePatterns(files []string, gi *ignore.GitIgnore) bool {
	if gi == nil || len(files) == 0 {
		return false
	}
	for _, f := range files {
		if f == ".lineignore" {
			return false
		}
		if !gi.MatchesPath(f) {
			return false
		}
	}
	return true
}

// allFilesIgnored returns true if every file changed in the given commit
// matches the ignore patterns.
func allFilesIgnored(repo *gitops.Repo, hash string, gi *ignore.GitIgnore) bool {
	if gi == nil {
		return false
	}
	files, err := repo.FilesChangedInCommit(hash)
	if err != nil || len(files) == 0 {
		return false
	}
	return filesMatchIgnorePatterns(files, gi)
}

// allCommitsSkipped returns true if every commit between lastSeen and head
// contains a skip marker ([skip ci], [ci skip], [skip line], [line skip]).
// When skipAgentCommits is true, commits with a Triggered-By trailer are also
// treated as skippable. This is used for concerns watching external branches
// (like main) where agent commits arrived via rebase and should not re-trigger.
// Returns false if there are no commits or if any commit lacks a skip marker.
func allCommitsSkipped(repo *gitops.Repo, lastSeen, head string, skipAgentCommits bool, gi *ignore.GitIgnore) bool {
	commits, err := repo.CommitsBetween(lastSeen, head)
	if err != nil || len(commits) == 0 {
		return false
	}
	allSkipped := true
	err = forEachCommitMessage(repo, commits, func(hash, msg string) error {
		if hasSkipMarker(msg) {
			return nil
		}
		if skipAgentCommits && isAgentCommit(msg) {
			return nil
		}
		if allFilesIgnored(repo, hash, gi) {
			return nil
		}
		allSkipped = false
		return nil
	})
	return err == nil && allSkipped
}

// hasSkipMarker checks if a commit message contains a recognized skip marker.
func hasSkipMarker(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "[skip ci]") ||
		strings.Contains(lower, "[ci skip]") ||
		strings.Contains(lower, "[skip line]") ||
		strings.Contains(lower, "[line skip]")
}

// isAgentCommit checks if a commit message was produced by the assembly-line agent
// or by a known AI coding tool. Agent commits contain a "Triggered-By:" trailer
// (see commitChanges). As a safety net, commits with a Co-Authored-By line
// matching known AI tool signatures are also treated as agent commits.
func isAgentCommit(msg string) bool {
	for _, line := range strings.Split(msg, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Triggered-By:") {
			return true
		}
		if strings.HasPrefix(trimmed, "Co-Authored-By:") && containsAISignature(trimmed) {
			return true
		}
	}
	return false
}

// containsAISignature checks if a Co-Authored-By line contains a known
// AI coding tool signature (case-insensitive).
func containsAISignature(line string) bool {
	lower := strings.ToLower(line)
	signatures := []string{
		"claude",
		"copilot",
		"cursor",
		"noreply@anthropic.com",
		"noreply@github.com",
	}
	for _, sig := range signatures {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}

// WatchesExternalBranch returns true if the concern watches a branch that is
// not another concern's output — i.e., it watches an external branch like "main".
func WatchesExternalBranch(cfg *config.Config, concern config.Concern) bool {
	return !cfg.HasConcern(concern.Watches)
}

// topologicalLevels groups concerns into levels for parallel execution.
// Level 0 = roots (watch external branches), Level 1 = depends only on level 0, etc.
func topologicalLevels(cfg *config.Config) [][]config.Concern {
	nameSet := cfg.BuildNameSet()

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
