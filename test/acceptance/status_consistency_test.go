package acceptance_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// statusSnapshot captures all four status sources at a point in time.
type statusSnapshot struct {
	timestamp      time.Time
	statusFile     *concernStatus
	statuslineData *statuslineOutput
	statusText     string
	statuslineText string
}

// stateOrder maps statusline-data states to ordinals for transition validation.
// Returns -1 for states outside the normal processing progression (e.g., failed).
func stateOrder(state string) int {
	switch state {
	case "unknown", "pending":
		return 0
	case "change_detected":
		return 1
	case "agent_running":
		return 2
	case "committing":
		return 3
	case "idle":
		return 4
	default:
		return -1
	}
}

var _ = Describe("status tracking consistency", func() {
	var tmpDir string
	var repoDir string
	var configPath string

	BeforeEach(func() {
		tmpDir, repoDir = setupTestRepo("detergent-status-consistency-*")

		// Add a README for the agent to modify
		writeFile(filepath.Join(repoDir, "README.md"), "# Test Project\n")
		runGit(repoDir, "add", "README.md")
		runGit(repoDir, "commit", "-m", "add README")

		configPath = filepath.Join(repoDir, "detergent.yaml")
		writeFile(configPath, `
agent:
  command: "sh"
  args: ["-c", "sleep 2 && echo 'Line added by agent' >> README.md"]

settings:
  poll_interval: 1s

concerns:
  - name: readme
    watches: main
    prompt: "Add a line to the README"
`)
	})

	AfterEach(func() {
		cleanupTestRepo(repoDir, tmpDir)
	})

	// captureSnap reads all four status sources and returns a snapshot.
	captureSnap := func() statusSnapshot {
		snap := statusSnapshot{timestamp: time.Now()}

		// 1. Read raw status JSON file
		statusPath := filepath.Join(repoDir, ".detergent", "status", "readme.json")
		if data, err := os.ReadFile(statusPath); err == nil {
			var s concernStatus
			if json.Unmarshal(data, &s) == nil {
				snap.statusFile = &s
			}
		}

		// 2. Run statusline-data command (JSON)
		cmd := exec.Command(binaryPath, "statusline-data", "--path", configPath)
		if out, err := cmd.CombinedOutput(); err == nil {
			var result statuslineOutput
			if json.Unmarshal(out, &result) == nil {
				snap.statuslineData = &result
			}
		}

		// 3. Run status command (text)
		cmd = exec.Command(binaryPath, "status", "--path", configPath)
		if out, err := cmd.CombinedOutput(); err == nil {
			snap.statusText = string(out)
		}

		// 4. Run statusline command (ANSI-stripped text)
		cmd = exec.Command(binaryPath, "statusline")
		cmd.Stdin = strings.NewReader(`{"cwd":"` + repoDir + `"}`)
		if out, err := cmd.CombinedOutput(); err == nil {
			snap.statuslineText = stripANSI(string(out))
		}

		return snap
	}

	// getState returns the canonical state from statusline-data for the "readme" concern.
	getState := func(snap statusSnapshot) string {
		if snap.statuslineData == nil {
			return ""
		}
		for _, c := range snap.statuslineData.Concerns {
			if c.Name == "readme" {
				return c.State
			}
		}
		return ""
	}

	// checkConsistency verifies all four status sources agree for the given snapshot.
	// Returns nil if consistent, or an error describing the inconsistency.
	checkConsistency := func(snap statusSnapshot) error {
		state := getState(snap)
		if state == "" {
			return fmt.Errorf("no canonical state found in statusline-data")
		}

		var issues []string
		check := func(cond bool, msg string) {
			if !cond {
				issues = append(issues, msg)
			}
		}

		switch state {
		case "unknown":
			check(snap.statusFile == nil,
				fmt.Sprintf("expected no status file for unknown, got %+v", snap.statusFile))
			check(strings.Contains(snap.statusText, "pending"),
				"expected status text to contain 'pending'")
			check(strings.Contains(snap.statuslineText, "·"),
				"expected statusline to contain '·'")

		case "idle":
			check(snap.statusFile != nil, "expected status file for idle")
			if snap.statusFile != nil {
				check(snap.statusFile.State == "idle",
					fmt.Sprintf("expected file state=idle, got %s", snap.statusFile.State))
			}
			check(strings.Contains(snap.statusText, "caught up"),
				"expected status text to contain 'caught up'")
			check(strings.Contains(snap.statuslineText, "✓"),
				"expected statusline to contain '✓'")

		case "pending":
			// statusline-data normalizes idle+behind_head to pending;
			// the raw status file should still show idle
			check(snap.statusFile != nil, "expected status file for pending")
			if snap.statusFile != nil {
				check(snap.statusFile.State == "idle",
					fmt.Sprintf("expected file state=idle for pending, got %s", snap.statusFile.State))
			}
			check(strings.Contains(snap.statusText, "pending"),
				"expected status text to contain 'pending'")
			check(strings.Contains(snap.statuslineText, "◯"),
				"expected statusline to contain '◯'")

		case "change_detected":
			check(snap.statusFile != nil, "expected status file for change_detected")
			if snap.statusFile != nil {
				check(snap.statusFile.State == "change_detected",
					fmt.Sprintf("expected file state=change_detected, got %s", snap.statusFile.State))
			}
			check(strings.Contains(snap.statusText, "change detected"),
				"expected status text to contain 'change detected'")
			check(strings.Contains(snap.statuslineText, "◎"),
				"expected statusline to contain '◎'")

		case "agent_running":
			check(snap.statusFile != nil, "expected status file for agent_running")
			if snap.statusFile != nil {
				check(snap.statusFile.State == "agent_running",
					fmt.Sprintf("expected file state=agent_running, got %s", snap.statusFile.State))
			}
			check(strings.Contains(snap.statusText, "agent running"),
				"expected status text to contain 'agent running'")
			check(strings.Contains(snap.statuslineText, "⟳"),
				"expected statusline to contain '⟳'")

		case "committing":
			check(snap.statusFile != nil, "expected status file for committing")
			if snap.statusFile != nil {
				check(snap.statusFile.State == "committing",
					fmt.Sprintf("expected file state=committing, got %s", snap.statusFile.State))
			}
			check(strings.Contains(snap.statusText, "committing"),
				"expected status text to contain 'committing'")
			check(strings.Contains(snap.statuslineText, "⟳"),
				"expected statusline to contain '⟳'")

		case "failed":
			check(snap.statusFile != nil, "expected status file for failed")
			check(strings.Contains(snap.statusText, "failed") || strings.Contains(snap.statusText, "stale"),
				"expected status text to contain 'failed' or 'stale'")
			check(strings.Contains(snap.statuslineText, "✗"),
				"expected statusline to contain '✗'")
		}

		if len(issues) > 0 {
			return fmt.Errorf("state=%s: %s\n  statusText=%q\n  statuslineText=%q",
				state, strings.Join(issues, "; "),
				snap.statusText, snap.statuslineText)
		}
		return nil
	}

	// inferStatuslineState determines the implied state from statusline symbols.
	inferStatuslineState := func(text string) string {
		if strings.Contains(text, "✓") {
			return "idle"
		}
		if strings.Contains(text, "◎") {
			return "change_detected"
		}
		if strings.Contains(text, "⟳") {
			return "agent_running" // also used for committing, close enough for ordering
		}
		if strings.Contains(text, "✗") {
			return "failed"
		}
		if strings.Contains(text, "◯") {
			return "pending"
		}
		if strings.Contains(text, "·") {
			return "unknown"
		}
		return ""
	}

	// inferStatusTextState determines the implied state from status command text.
	inferStatusTextState := func(text string) string {
		// Check specific phrases first, then generic ones
		if strings.Contains(text, "caught up") {
			return "idle"
		}
		if strings.Contains(text, "change detected") {
			return "change_detected"
		}
		if strings.Contains(text, "agent running") {
			return "agent_running"
		}
		if strings.Contains(text, "committing") {
			return "committing"
		}
		if strings.Contains(text, "failed") || strings.Contains(text, "stale") {
			return "failed"
		}
		if strings.Contains(text, "pending") {
			return "pending" // covers both "never processed" and "behind head"
		}
		return ""
	}

	// isRacingSnapshot returns true if a consistency error is likely due to a
	// state transition occurring between the sequential reads of the four sources.
	// Compares rendered outputs (statusline-data, status text, statusline text)
	// and allows a spread of 1 in the state ordering. The raw status file is
	// excluded because statusline-data applies normalizations (idle+behind→pending)
	// that make raw vs canonical comparison misleading.
	isRacingSnapshot := func(snap statusSnapshot) bool {
		canonState := getState(snap)
		canonOrd := stateOrder(canonState)
		if canonOrd < 0 {
			return false
		}

		observedOrders := []int{canonOrd}

		if slState := inferStatuslineState(snap.statuslineText); slState != "" {
			if ord := stateOrder(slState); ord >= 0 {
				observedOrders = append(observedOrders, ord)
			}
		}

		if stState := inferStatusTextState(snap.statusText); stState != "" {
			if ord := stateOrder(stState); ord >= 0 {
				observedOrders = append(observedOrders, ord)
			}
		}

		minOrd, maxOrd := observedOrders[0], observedOrders[0]
		for _, o := range observedOrders[1:] {
			if o < minOrd {
				minOrd = o
			}
			if o > maxOrd {
				maxOrd = o
			}
		}
		return maxOrd-minOrd <= 1
	}

	// pollCycle polls every 200ms, collecting snapshots through a processing cycle.
	// It waits for the state to leave leaveState (if non-empty), then waits for
	// targetState to be reached. Returns all collected snapshots.
	pollCycle := func(leaveState, targetState string, timeout time.Duration) []statusSnapshot {
		deadline := time.Now().Add(timeout)
		var snapshots []statusSnapshot
		leftInitial := leaveState == ""

		for time.Now().Before(deadline) {
			snap := captureSnap()
			snapshots = append(snapshots, snap)
			state := getState(snap)

			if !leftInitial && state != leaveState {
				leftInitial = true
			}
			if leftInitial && state == targetState {
				return snapshots
			}

			time.Sleep(200 * time.Millisecond)
		}

		// Timeout — capture one final snapshot for diagnostics
		snap := captureSnap()
		snapshots = append(snapshots, snap)
		Fail(fmt.Sprintf("timed out in pollCycle (leave=%q, target=%q, last=%q, %d snapshots)",
			leaveState, targetState, getState(snap), len(snapshots)))
		return snapshots
	}

	// validateTransitions checks that state transitions within a set of snapshots
	// are monotonically forward (no backwards jumps within a processing cycle).
	validateTransitions := func(snapshots []statusSnapshot) {
		maxOrd := -1
		for i, snap := range snapshots {
			state := getState(snap)
			ord := stateOrder(state)
			if ord < 0 {
				continue // skip failed/unknown states for ordering purposes
			}
			if ord < maxOrd {
				Fail(fmt.Sprintf("backward state transition at snapshot %d: state=%s (order=%d) after max order=%d",
					i, state, ord, maxOrd))
			}
			if ord > maxOrd {
				maxOrd = ord
			}
		}
	}

	// validateCycleConsistency checks consistency across a set of polling snapshots.
	// Allows racing (adjacent state differences from sequential reads) but fails
	// on genuine inconsistencies.
	validateCycleConsistency := func(snapshots []statusSnapshot, cycleName string) {
		var hardErrors []string
		racingCount := 0
		for i, snap := range snapshots {
			if err := checkConsistency(snap); err != nil {
				if isRacingSnapshot(snap) {
					racingCount++
					continue
				}
				hardErrors = append(hardErrors,
					fmt.Sprintf("  snapshot %d (state=%s): %v", i, getState(snap), err))
			}
		}
		if racingCount > 0 {
			GinkgoWriter.Printf("%s: %d/%d snapshots had racing reads (adjacent state transitions)\n",
				cycleName, racingCount, len(snapshots))
		}
		if len(hardErrors) > 0 {
			Fail(fmt.Sprintf("consistency errors during %s:\n%s", cycleName, strings.Join(hardErrors, "\n")))
		}
	}

	It("all status sources agree through a full daemon lifecycle", func() {
		// ── PHASE 1: Pre-daemon — all sources should show unknown ──
		snap := captureSnap()
		Expect(checkConsistency(snap)).To(Succeed(), "pre-daemon consistency")
		Expect(getState(snap)).To(Equal("unknown"))

		// ── PHASE 2: Start daemon, poll through first processing cycle ──
		cmd := exec.Command(binaryPath, "run", "--path", configPath)
		cmd.Dir = repoDir
		var outputBuf strings.Builder
		cmd.Stdout = &outputBuf
		cmd.Stderr = &outputBuf

		err := cmd.Start()
		Expect(err).NotTo(HaveOccurred())

		daemonStopped := false
		defer func() {
			if !daemonStopped {
				cmd.Process.Signal(syscall.SIGINT)
				cmd.Wait()
			}
		}()

		snapshots := pollCycle("", "idle", 30*time.Second)

		// Validate consistency and transitions for first cycle
		validateCycleConsistency(snapshots, "first cycle")
		validateTransitions(snapshots)

		// Log observed states
		seenStates := make(map[string]bool)
		for _, s := range snapshots {
			seenStates[getState(s)] = true
		}
		GinkgoWriter.Printf("First cycle states observed: %v (%d snapshots)\n", seenStates, len(snapshots))

		// Soft assertion: the sleep 2 agent should give us time to see intermediate states
		if !seenStates["change_detected"] && !seenStates["agent_running"] {
			GinkgoWriter.Println("WARNING: no intermediate states observed during first cycle (timing-dependent)")
		}

		// Verify final state: idle with last_result=modified
		finalSnap := snapshots[len(snapshots)-1]
		Expect(checkConsistency(finalSnap)).To(Succeed(), "first cycle final snapshot")
		for _, c := range finalSnap.statuslineData.Concerns {
			if c.Name == "readme" {
				Expect(c.LastResult).To(Equal("modified"),
					"agent appended to README, expected last_result=modified")
			}
		}

		// ── PHASE 3: Make a new commit to trigger second processing cycle ──
		writeFile(filepath.Join(repoDir, "newfile.txt"), "new content\n")
		runGit(repoDir, "add", "newfile.txt")
		runGit(repoDir, "commit", "-m", "second commit")

		// ── PHASE 4: Poll through second processing cycle ──
		// Wait for state to leave idle (daemon detects new commit), then wait for idle again
		snapshots2 := pollCycle("idle", "idle", 30*time.Second)

		validateCycleConsistency(snapshots2, "second cycle")

		// For transition validation, skip snapshots that are still idle (pre-detection)
		// and only validate the processing portion
		var processingSnapshots []statusSnapshot
		processing := false
		for _, s := range snapshots2 {
			state := getState(s)
			if !processing && state != "idle" {
				processing = true
			}
			if processing {
				processingSnapshots = append(processingSnapshots, s)
			}
		}
		if len(processingSnapshots) > 0 {
			validateTransitions(processingSnapshots)
		}

		seenStates2 := make(map[string]bool)
		for _, s := range snapshots2 {
			seenStates2[getState(s)] = true
		}
		GinkgoWriter.Printf("Second cycle states observed: %v (%d snapshots)\n", seenStates2, len(snapshots2))

		// Verify second cycle final state
		finalSnap2 := snapshots2[len(snapshots2)-1]
		Expect(checkConsistency(finalSnap2)).To(Succeed(), "second cycle final snapshot")
		Expect(getState(finalSnap2)).To(Equal("idle"))

		// Verify pending detection worked: we should have seen pending or change_detected
		// at some point during the second cycle
		Expect(seenStates2).To(SatisfyAny(
			HaveKey("pending"),
			HaveKey("change_detected"),
			HaveKey("agent_running"),
		), "should have observed the daemon detecting and processing the new commit")

		// ── PHASE 5: Stop daemon and verify final state ──
		cmd.Process.Signal(syscall.SIGINT)
		err = cmd.Wait()
		daemonStopped = true
		Expect(err).NotTo(HaveOccurred(), "daemon output: %s", outputBuf.String())

		// Final consistency check after daemon shutdown
		snap = captureSnap()
		Expect(checkConsistency(snap)).To(Succeed(), "post-daemon consistency")
		Expect(getState(snap)).To(Equal("idle"))
	})
})
