package tmux_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/re-cinq/assembly-line/internal/tmux"
)

func skipIfNoTmux(t *testing.T) {
	t.Helper()
	if !tmux.Available() {
		t.Skip("tmux not installed")
	}
}

func TestAvailable(t *testing.T) {
	// Available should return a consistent result and not panic.
	result := tmux.Available()
	_, err := exec.LookPath("tmux")
	want := err == nil
	if result != want {
		t.Fatalf("Available() = %v, want %v", result, want)
	}
}

func TestSessionName(t *testing.T) {
	name := tmux.SessionName("/some/repo/path", "review")
	if !strings.HasPrefix(name, "line-") {
		t.Fatalf("SessionName should start with 'line-', got %q", name)
	}
	if !strings.HasSuffix(name, "-review") {
		t.Fatalf("SessionName should end with '-review', got %q", name)
	}
	// Deterministic: same inputs → same output
	name2 := tmux.SessionName("/some/repo/path", "review")
	if name != name2 {
		t.Fatalf("SessionName not deterministic: %q != %q", name, name2)
	}
	// Different inputs → different output
	name3 := tmux.SessionName("/other/repo", "review")
	if name == name3 {
		t.Fatalf("SessionName should differ for different repos")
	}
	// 8 char hash: "line-" (5) + 8 + "-" (1) + station
	parts := strings.SplitN(name, "-", 3)
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts in %q, got %d", name, len(parts))
	}
	if len(parts[1]) != 8 {
		t.Fatalf("hash portion should be 8 chars, got %d: %q", len(parts[1]), parts[1])
	}
}

func TestSessionLifecycle(t *testing.T) {
	skipIfNoTmux(t)

	session := "line-test-lifecycle"
	// Ensure clean state
	_ = tmux.KillSession(session)

	// Should not exist yet
	if tmux.HasSession(session) {
		t.Fatal("session should not exist before creation")
	}

	// Create session running a simple command that stays alive
	err := tmux.NewSession(session, os.TempDir(), "sleep 60")
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer tmux.KillSession(session)

	// Should exist now
	if !tmux.HasSession(session) {
		t.Fatal("session should exist after creation")
	}

	// Kill and verify gone
	if err := tmux.KillSession(session); err != nil {
		t.Fatalf("KillSession failed: %v", err)
	}
	if tmux.HasSession(session) {
		t.Fatal("session should not exist after kill")
	}
}

func TestSetOption(t *testing.T) {
	skipIfNoTmux(t)

	session := "line-test-setopt"
	_ = tmux.KillSession(session)

	err := tmux.NewSession(session, os.TempDir(), "sleep 60")
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer tmux.KillSession(session)

	// Setting remain-on-exit should not error
	if err := tmux.SetOption(session, "remain-on-exit", "on"); err != nil {
		t.Fatalf("SetOption failed: %v", err)
	}
}

func TestPanePID(t *testing.T) {
	skipIfNoTmux(t)

	session := "line-test-panepid"
	_ = tmux.KillSession(session)

	err := tmux.NewSession(session, os.TempDir(), "sleep 60")
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer tmux.KillSession(session)

	pid, err := tmux.PanePID(session)
	if err != nil {
		t.Fatalf("PanePID failed: %v", err)
	}
	if pid <= 0 {
		t.Fatalf("PanePID returned invalid pid: %d", pid)
	}
}

func TestPaneStatus(t *testing.T) {
	skipIfNoTmux(t)

	session := "line-test-panestatus"
	_ = tmux.KillSession(session)

	// Create a session that runs a short command (NewSession sets remain-on-exit)
	err := tmux.NewSession(session, os.TempDir(), "true")
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer tmux.KillSession(session)

	// Wait for pane to die (command "true" exits immediately)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		dead, exitCode, err := tmux.PaneStatus(session)
		if err != nil {
			t.Fatalf("PaneStatus failed: %v", err)
		}
		if dead {
			if exitCode != 0 {
				t.Fatalf("expected exit code 0 for 'true', got %d", exitCode)
			}
			return // success
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("pane did not become dead within timeout")
}

func TestPaneStatusNonZero(t *testing.T) {
	skipIfNoTmux(t)

	session := "line-test-panestatus-nz"
	_ = tmux.KillSession(session)

	err := tmux.NewSession(session, os.TempDir(), "exit 42")
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer tmux.KillSession(session)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		dead, exitCode, err := tmux.PaneStatus(session)
		if err != nil {
			t.Fatalf("PaneStatus failed: %v", err)
		}
		if dead {
			if exitCode != 42 {
				t.Fatalf("expected exit code 42, got %d", exitCode)
			}
			return // success
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("pane did not become dead within timeout")
}

func TestPipePane(t *testing.T) {
	skipIfNoTmux(t)

	session := "line-test-pipepane"
	_ = tmux.KillSession(session)

	logFile := filepath.Join(os.TempDir(), "line-test-pipepane.log")
	os.Remove(logFile)
	defer os.Remove(logFile)

	// Run a command that stays alive long enough for pipe-pane setup
	err := tmux.NewSession(session, os.TempDir(), "sleep 30")
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer tmux.KillSession(session)

	// Set up pipe-pane, then send keys to produce output
	if err := tmux.PipePane(session, "cat >> "+logFile); err != nil {
		t.Fatalf("PipePane failed: %v", err)
	}
	// Use send-keys to type a command that produces output after pipe-pane is active
	if err := tmux.SendKeys(session, "echo 'hello from pipe-pane'"); err != nil {
		t.Fatalf("SendKeys failed: %v", err)
	}

	// Wait for output to appear in log file
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(logFile)
		if err == nil && strings.Contains(string(data), "hello from pipe-pane") {
			return // success
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("pipe-pane output did not appear in log file within timeout")
}

func TestCapturePaneLines(t *testing.T) {
	skipIfNoTmux(t)

	session := "line-test-capture"
	_ = tmux.KillSession(session)

	err := tmux.NewSession(session, os.TempDir(), "echo 'capture-test-output'; sleep 5")
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer tmux.KillSession(session)

	// Give the echo a moment to render
	time.Sleep(500 * time.Millisecond)

	output, err := tmux.CapturePaneLines(session, 15)
	if err != nil {
		t.Fatalf("CapturePaneLines failed: %v", err)
	}
	if !strings.Contains(output, "capture-test-output") {
		t.Fatalf("CapturePaneLines output should contain 'capture-test-output', got: %q", output)
	}
}

func TestCleanStaleSessions(t *testing.T) {
	skipIfNoTmux(t)

	// Create a session with our naming convention
	repoDir := "/tmp/line-test-clean-stale"
	session := tmux.SessionName(repoDir, "teststn")
	_ = tmux.KillSession(session)

	err := tmux.NewSession(session, os.TempDir(), "sleep 60")
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	if !tmux.HasSession(session) {
		t.Fatal("session should exist before clean")
	}

	// CleanStaleSessions for this repo should kill it
	if err := tmux.CleanStaleSessions(repoDir); err != nil {
		t.Fatalf("CleanStaleSessions failed: %v", err)
	}

	if tmux.HasSession(session) {
		t.Fatal("session should be gone after CleanStaleSessions")
	}
}

func TestKillSessionIdempotent(t *testing.T) {
	skipIfNoTmux(t)

	// Killing a non-existent session should not error
	err := tmux.KillSession("line-test-nonexistent-session")
	if err != nil {
		t.Fatalf("KillSession on non-existent session should not error, got: %v", err)
	}
}
