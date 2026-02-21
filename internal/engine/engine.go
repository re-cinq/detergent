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

// LogManager manages per-station log files for agent output.
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

// getLogFile returns the log file for a station, creating it if necessary.
// Log files are stored in the system temp directory with the pattern line-<station>.log.
func (lm *LogManager) getLogFile(stationName string) (*os.File, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if f, ok := lm.files[stationName]; ok {
		return f, nil
	}

	logPath := LogPathFor(stationName)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening log file %s: %w", logPath, err)
	}

	lm.files[stationName] = f
	return f, nil
}

// truncateLogFile truncates the log file for a station to clear old logs from previous runs.
func (lm *LogManager) truncateLogFile(stationName string) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Close and remove the cached file handle if it exists
	if f, ok := lm.files[stationName]; ok {
		f.Close()
		delete(lm.files, stationName)
	}

	logPath := LogPathFor(stationName)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("truncating log file %s: %w", logPath, err)
	}

	lm.files[stationName] = f
	return nil
}

// LogPath returns the log file path pattern for display purposes.
func LogPath() string {
	return filepath.Join(os.TempDir(), "line-<station>.log")
}

// LogPathFor returns the log file path for a specific station.
func LogPathFor(stationName string) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("line-%s.log", stationName))
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

// RunOnce processes each station once and returns.
// Independent stations at the same level run in parallel.
// Individual station failures are logged but don't stop other stations.
// Creates a temporary LogManager that is closed after processing.
func RunOnce(cfg *config.Config, repoDir string) error {
	logMgr := NewLogManager()
	defer logMgr.Close()
	return RunOnceWithLogs(cfg, repoDir, logMgr)
}

// shouldSkipStation checks if a station should be skipped due to upstream failures.
// If upstream failed, it writes a skip status and returns true.
func shouldSkipStation(repoDir string, c config.Station, failed *failedSet) bool {
	if failed.has(c.Watches) {
		skipUpstreamFailed(repoDir, c.Name, os.Getpid())
		return true
	}
	return false
}

// processStationAndTrackFailure processes a station and tracks failures in the failedSet.
func processStationAndTrackFailure(cfg *config.Config, repo *gitops.Repo, repoDir string, c config.Station, logMgr *LogManager, failed *failedSet) {
	if err := processStation(cfg, repo, repoDir, c, logMgr); err != nil {
		fileutil.LogError("station %s failed: %s", c.Name, err)
		failed.set(c.Name)
	}
}

// RunOnceWithLogs processes each station once using the provided LogManager.
// The LogManager is not closed; the caller is responsible for closing it.
func RunOnceWithLogs(cfg *config.Config, repoDir string, logMgr *LogManager) error {
	// Clear any stale active statuses from a previous interrupted run.
	stationNames := make([]string, len(cfg.Stations))
	for i, c := range cfg.Stations {
		stationNames[i] = c.Name
	}
	ResetActiveStatuses(repoDir, stationNames)

	repo := gitops.NewRepo(repoDir)
	repo.EnsureIdentity()

	levels := topologicalLevels(cfg)
	failed := &failedSet{m: make(map[string]bool)}

	for _, level := range levels {
		if len(level) == 1 {
			// Single station: run directly (no goroutine overhead)
			c := level[0]
			if !shouldSkipStation(repoDir, c, failed) {
				processStationAndTrackFailure(cfg, repo, repoDir, c, logMgr, failed)
			}
		} else {
			// Multiple independent stations: run in parallel
			var wg sync.WaitGroup
			for _, c := range level {
				if shouldSkipStation(repoDir, c, failed) {
					continue
				}
				wg.Add(1)
				go func(station config.Station) {
					defer wg.Done()
					processStationAndTrackFailure(cfg, repo, repoDir, station, logMgr, failed)
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

// stationContext holds the execution context for processing a single station.
// It bundles frequently-used parameters to reduce function signatures.
type stationContext struct {
	repoDir     string
	stationName string
	startedAt   string
	head        string
	pid         int
}

// fail writes a failed status and returns a wrapped error.
func (ctx *stationContext) fail(origErr error, wrappedErr error) error {
	return processStationFailed(ctx.repoDir, ctx.stationName, ctx.startedAt,
		ctx.head, ctx.pid, origErr, wrappedErr)
}

func processStation(cfg *config.Config, repo *gitops.Repo, repoDir string, station config.Station, logMgr *LogManager) error {
	pid := os.Getpid()
	watchedBranch := ResolveWatchedBranch(cfg, station)

	// Get current HEAD of watched branch
	head, err := repo.HeadCommit(watchedBranch)
	if err != nil {
		return fmt.Errorf("getting HEAD of %s: %w", watchedBranch, err)
	}

	// Check last-seen
	lastSeen, err := LastSeen(repoDir, station.Name)
	if err != nil {
		return err
	}
	if lastSeen == head {
		// Nothing new — preserve last_result from previous run
		writeIdleStatus(repoDir, station.Name, pid)
		return nil // nothing new
	}

	// Check if all new commits have skip markers (or agent commits on external branches)
	skipAgentCommits := WatchesExternalBranch(cfg, station)
	gi := loadIgnorePatterns(repoDir)
	if allCommitsSkipped(repo, lastSeen, head, skipAgentCommits, gi) {
		// Advance last-seen so we don't re-check these commits
		if err := SetLastSeen(repoDir, station.Name, head); err != nil {
			return fmt.Errorf("updating last-seen after skip: %w", err)
		}
		writeIdleStatus(repoDir, station.Name, pid)
		return nil
	}

	// Create execution context to reduce parameter passing
	ctx := &stationContext{
		repoDir:     repoDir,
		stationName: station.Name,
		startedAt:   nowRFC3339(),
		head:        head,
		pid:         pid,
	}

	// Write change-detected status
	writeChangeDetectedStatus(ctx.repoDir, ctx.stationName, ctx.startedAt, ctx.head, ctx.pid)

	outputBranch := cfg.Settings.BranchPrefix + station.Name

	// Ensure output branch exists
	if !repo.BranchExists(outputBranch) {
		if err := repo.CreateBranch(outputBranch, watchedBranch); err != nil {
			return ctx.fail(err, fmt.Errorf("creating output branch %s: %w", outputBranch, err))
		}
	}

	// Ensure worktree exists
	wtPath := gitops.WorktreePath(repoDir, cfg.Settings.BranchPrefix, station.Name)
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		if err := fileutil.EnsureDir(filepath.Dir(wtPath)); err != nil {
			return ctx.fail(err, fmt.Errorf("creating worktree directory: %w", err))
		}
		if err := repo.CreateWorktree(wtPath, outputBranch); err != nil {
			return ctx.fail(err, fmt.Errorf("creating worktree: %w", err))
		}
	}

	// Rebase output branch onto watched branch so prior station
	// commits sit on top of the latest upstream state.
	if err := rebaseWorktree(wtPath, watchedBranch); err != nil {
		return ctx.fail(err, fmt.Errorf("rebasing %s onto %s: %w", outputBranch, watchedBranch, err))
	}

	// Assemble context
	context, err := assembleContext(repo, cfg, station, lastSeen, head)
	if err != nil {
		return ctx.fail(err, fmt.Errorf("assembling context: %w", err))
	}

	// Truncate log file to clear old logs from previous runs
	if err := logMgr.truncateLogFile(station.Name); err != nil {
		return ctx.fail(err, fmt.Errorf("truncating log file: %w", err))
	}

	// Get log file for this station (after truncate, this returns the fresh file)
	logFile, err := logMgr.getLogFile(station.Name)
	if err != nil {
		return ctx.fail(err, fmt.Errorf("getting log file: %w", err))
	}

	// Write commit context header to log file
	header := fmt.Sprintf("--- Processing %s at %s ---\n", head, time.Now().UTC().Format(time.RFC3339))
	if _, err := logFile.WriteString(header); err != nil {
		return ctx.fail(err, fmt.Errorf("writing log header: %w", err))
	}

	// Write agent-started status
	writeAgentRunningStatus(ctx.repoDir, ctx.stationName, ctx.startedAt, ctx.head, ctx.pid)

	// Snapshot worktree HEAD before agent runs so we can detect rogue commits
	wtRepo := gitops.NewRepo(wtPath)
	preAgentHead, err := wtRepo.HeadCommit("HEAD")
	if err != nil {
		return ctx.fail(err, fmt.Errorf("snapshotting worktree HEAD: %w", err))
	}

	// Invoke agent in worktree
	if err := invokeAgent(cfg, station, wtPath, context, logFile); err != nil {
		return ctx.fail(err, fmt.Errorf("invoking agent: %w", err))
	}

	// Soft-reset any commits the agent made directly — we need the file
	// changes but will create a proper commit with Triggered-By trailers.
	postAgentHead, err := wtRepo.HeadCommit("HEAD")
	if err != nil {
		return ctx.fail(err, fmt.Errorf("checking worktree HEAD after agent: %w", err))
	}
	if postAgentHead != preAgentHead {
		fileutil.LogError("station %s: agent made direct commits — soft-resetting to preserve changes", station.Name)
		if err := wtRepo.ResetSoft(preAgentHead); err != nil {
			return ctx.fail(err, fmt.Errorf("soft-resetting agent commits: %w", err))
		}
	}

	// Write agent-succeeded status
	writeCommittingStatus(ctx.repoDir, ctx.stationName, ctx.startedAt, ctx.head, ctx.pid)

	// Check for changes and commit
	changed, err := commitChanges(wtPath, station, head)
	if err != nil {
		return ctx.fail(err, fmt.Errorf("committing changes: %w", err))
	}

	if !changed {
		// Branch already at or ahead of watched after rebase — just add notes
		commits, _ := repo.CommitsBetween(lastSeen, head)
		noteMsg := fmt.Sprintf("[%s] Reviewed, no changes needed", strings.ToUpper(station.Name))
		for _, hash := range commits {
			_ = repo.AddNote(hash, noteMsg)
		}
	}

	// Update last-seen
	if err := SetLastSeen(repoDir, station.Name, head); err != nil {
		return ctx.fail(err, fmt.Errorf("updating last-seen marker: %w", err))
	}

	// Write idle status with result
	result := ResultNoop
	if changed {
		result = ResultModified
	}
	writeIdleWithResultStatus(ctx.repoDir, ctx.stationName, ctx.startedAt, nowRFC3339(), ctx.head, result, ctx.pid)

	return nil
}

// getLastResult retrieves the LastResult from the previous status, or "" if not found.
func getLastResult(repoDir, stationName string) string {
	prevStatus, _ := ReadStatus(repoDir, stationName)
	if prevStatus != nil {
		return prevStatus.LastResult
	}
	return ""
}

// statusUpdate holds optional fields for writing station status.
// Zero values are omitted from the written status.
type statusUpdate struct {
	state       string
	startedAt   string
	completedAt string
	headAtStart string
	lastResult  string
	errorMsg    string
	pid         int
}

// writeStatus writes a station status with the given fields.
// This consolidates all status-writing into a single helper.
func writeStatus(repoDir, stationName string, u statusUpdate) {
	status := &StationStatus{
		State:       u.state,
		StartedAt:   u.startedAt,
		CompletedAt: u.completedAt,
		HeadAtStart: u.headAtStart,
		LastResult:  u.lastResult,
		Error:       u.errorMsg,
		PID:         u.pid,
	}
	_ = WriteStatus(repoDir, stationName, status)
}

// writeChangeDetectedStatus writes a change-detected status.
func writeChangeDetectedStatus(repoDir, stationName, startedAt, head string, pid int) {
	writeStatus(repoDir, stationName, statusUpdate{
		state:       StateChangeDetected,
		startedAt:   startedAt,
		headAtStart: head,
		pid:         pid,
	})
}

// writeAgentRunningStatus writes an agent-running status.
func writeAgentRunningStatus(repoDir, stationName, startedAt, head string, pid int) {
	writeStatus(repoDir, stationName, statusUpdate{
		state:       StateAgentRunning,
		startedAt:   startedAt,
		headAtStart: head,
		pid:         pid,
	})
}

// writeCommittingStatus writes a committing status.
func writeCommittingStatus(repoDir, stationName, startedAt, head string, pid int) {
	writeStatus(repoDir, stationName, statusUpdate{
		state:       StateCommitting,
		startedAt:   startedAt,
		headAtStart: head,
		pid:         pid,
	})
}

// writeIdleWithResultStatus writes an idle status with a specific result.
func writeIdleWithResultStatus(repoDir, stationName, startedAt, completedAt, head, result string, pid int) {
	writeStatus(repoDir, stationName, statusUpdate{
		state:       StateIdle,
		startedAt:   startedAt,
		completedAt: completedAt,
		headAtStart: head,
		lastResult:  result,
		pid:         pid,
	})
}

// writeIdleStatus writes an idle status, preserving the previous LastResult.
func writeIdleStatus(repoDir, stationName string, pid int) {
	writeStatus(repoDir, stationName, statusUpdate{
		state:      StateIdle,
		lastResult: getLastResult(repoDir, stationName),
		pid:        pid,
	})
}

// writeFailedStatus writes a failed status with completion timestamp and error.
func writeFailedStatus(repoDir, stationName, startedAt, completedAt, head, errorMsg string, pid int) {
	writeStatus(repoDir, stationName, statusUpdate{
		state:       StateFailed,
		startedAt:   startedAt,
		completedAt: completedAt,
		headAtStart: head,
		errorMsg:    errorMsg,
		pid:         pid,
	})
}

// writeSkippedStatus writes a skipped status with the given error message.
func writeSkippedStatus(repoDir, stationName, errorMsg string, pid int) {
	writeStatus(repoDir, stationName, statusUpdate{
		state:    StateSkipped,
		errorMsg: errorMsg,
		pid:      pid,
	})
}

// skipUpstreamFailed logs and marks a station as skipped due to upstream failure.
func skipUpstreamFailed(repoDir, stationName string, pid int) {
	fileutil.LogError("skipping %s: upstream station failed", stationName)
	writeSkippedStatus(repoDir, stationName, "upstream station failed", pid)
}

// processStationFailed writes a failed status and returns the wrapped error.
func processStationFailed(repoDir, stationName, startedAt, head string, pid int, origErr, wrappedErr error) error {
	writeFailedStatus(repoDir, stationName, startedAt, nowRFC3339(), head, origErr.Error(), pid)
	return wrappedErr
}

func ResolveWatchedBranch(cfg *config.Config, station config.Station) string {
	// If the station watches another station, resolve to its output branch
	for _, c := range cfg.Stations {
		if c.Name == station.Watches {
			return cfg.Settings.BranchPrefix + c.Name
		}
	}
	// Otherwise it's an external branch name
	return station.Watches
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

func assembleContext(repo *gitops.Repo, cfg *config.Config, station config.Station, lastSeen, head string) (string, error) {
	commits, err := repo.CommitsBetween(lastSeen, head)
	if err != nil {
		return "", err
	}

	// When watching an external branch, filter out agent commits from context.
	// After a rebase, these are our own output coming back — not new work.
	skipAgent := WatchesExternalBranch(cfg, station)

	var sb strings.Builder
	sb.WriteString(cfg.ResolvePreamble(station) + "\n\n")
	sb.WriteString("# Station: " + station.Name + "\n\n")
	sb.WriteString("## Prompt\n\n")
	sb.WriteString(station.Prompt + "\n\n")
	sb.WriteString("## New commits to review\n\n")

	// List commit hashes and messages (no diffs — the agent can inspect
	// them via git in the worktree, keeping the prompt size bounded).
	var userCommits int
	err = forEachCommitMessage(repo, commits, func(hash, msg string) error {
		if skipAgent && isAgentCommit(msg) {
			return nil
		}
		sb.WriteString("- " + hash[:8] + " " + strings.SplitN(msg, "\n", 2)[0] + "\n")
		userCommits++
		return nil
	})
	if err != nil {
		return "", err
	}

	sb.WriteString("\n## How to inspect changes\n\n")
	if lastSeen != "" {
		sb.WriteString("To see all changes since the last review:\n")
		sb.WriteString("```\ngit diff " + lastSeen[:8] + ".." + head[:8] + "\n```\n\n")
	} else {
		sb.WriteString("This is the first review. To see changes in the most recent commit:\n")
		sb.WriteString("```\ngit diff " + head[:8] + "~1.." + head[:8] + "\n```\n\n")
	}
	sb.WriteString(fmt.Sprintf("There are %d new commit(s) to review. Use `git log` and `git diff` to inspect them as needed.\n", userCommits))

	return sb.String(), nil
}

// FilterEnv returns a copy of os.Environ() with variables matching any of the
// given prefixes removed. Prefixes should include the '=' suffix for exact matching
// (e.g., "CLAUDECODE=", not "CLAUDECODE").
func FilterEnv(excludePrefixes ...string) []string {
	result := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		skip := false
		for _, prefix := range excludePrefixes {
			if strings.HasPrefix(e, prefix) {
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

func invokeAgent(cfg *config.Config, station config.Station, worktreeDir, context string, output io.Writer) error {
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

	// Resolve command and args: per-station overrides take precedence over global
	agentCommand := cfg.Agent.Command
	if station.Command != "" {
		agentCommand = station.Command
	}
	agentArgs := cfg.Agent.Args
	if station.Args != nil {
		agentArgs = station.Args
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

	// Build a clean environment for the agent:
	// - Strip CLAUDECODE so Claude Code agents don't refuse to start
	//   when line is invoked from within a Claude Code session
	// - Set LINE_AGENT so post-commit hooks don't re-trigger
	cmd.Env = append(FilterEnv("CLAUDECODE="), "LINE_AGENT=1")
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

func commitChanges(worktreeDir string, station config.Station, triggeredBy string) (bool, error) {
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
		strings.ToUpper(station.Name), triggeredBy)

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
// treated as skippable. This is used for stations watching external branches
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

// isAgentCommit checks if a commit message was produced by the assembly-line
// runner. Agent commits are identified solely by the "Triggered-By:" trailer
// that commitChanges adds. Co-Authored-By lines are NOT checked because users
// working with AI coding tools (Claude Code, Copilot, Cursor) produce those
// on normal commits — treating them as agent commits would cause the station
// line to silently skip real work.
func isAgentCommit(msg string) bool {
	for _, line := range strings.Split(msg, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Triggered-By:") {
			return true
		}
	}
	return false
}

// WatchesExternalBranch returns true if the station watches a branch that is
// not another station's output — i.e., it watches an external branch like "main".
func WatchesExternalBranch(cfg *config.Config, station config.Station) bool {
	return !cfg.HasStation(station.Watches)
}

// topologicalLevels groups stations into levels for parallel execution.
// Level 0 = roots (watch external branches), Level 1 = depends only on level 0, etc.
func topologicalLevels(cfg *config.Config) [][]config.Station {
	nameSet := cfg.BuildNameSet()

	byName := make(map[string]config.Station)
	for _, c := range cfg.Stations {
		byName[c.Name] = c
	}

	// Compute level for each station
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
	for _, c := range cfg.Stations {
		l := computeLevel(c.Name)
		if l > maxLevel {
			maxLevel = l
		}
	}

	// Group by level
	result := make([][]config.Station, maxLevel+1)
	for _, c := range cfg.Stations {
		l := levels[c.Name]
		result[l] = append(result[l], c)
	}

	return result
}
