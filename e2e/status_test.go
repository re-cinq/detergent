package e2e_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("line status", func() {
	var dir string

	BeforeEach(func() {
		dir = tempRepo()
		writeDefaultConfig(dir)
	})

	// STAT-1: Shows watched branch, shortref, and dirty indicator
	It("shows watched branch with shortref and dirty indicator [STAT-1]", func() {
		out := lineOK(dir, "status")

		// Should show master branch with a short ref
		Expect(out).To(ContainSubstring("master"))
		// Should show a short ref (7 chars typically)
		ref := shortRef(dir)
		Expect(out).To(ContainSubstring(ref))
	})

	// STAT-1: Shows dirty indicator when working tree is dirty
	It("shows dirty indicator when working tree has changes [STAT-1]", func() {
		writeFile(dir, "dirty.txt", "uncommitted change\n")

		out := lineOK(dir, "status")
		Expect(out).To(ContainSubstring("dirty"))
	})

	// STAT-2: Station status indicators
	It("shows station status as pending when no run has occurred [STAT-2]", func() {
		out := lineOK(dir, "status")
		Expect(out).To(ContainSubstring("review"))
		Expect(out).To(ContainSubstring("pending"))
	})

	// STAT-2: Pending status is colour-coded yellow
	It("colour-codes pending status as yellow [STAT-2]", func() {
		out := lineOK(dir, "status")
		Expect(out).To(ContainSubstring("\033[93m"))
		Expect(out).To(ContainSubstring("pending"))
	})

	// STAT-2: Shows up to date after a run
	It("shows station as up to date after a successful run [STAT-2]", func() {
		agentScript := writeMockAgent(dir)
		writeConfig(dir, `agent:
  command: `+agentScript+`
  args: ["-p"]

settings:
  watches: master

stations:
  - name: review
    prompt: "Review code"
`)
		writeFile(dir, "code.go", "package main\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add code")

		lineOK(dir, "run")

		out := lineOK(dir, "status")
		Expect(out).To(ContainSubstring("up to date"))
		// STAT-2: up to date is green
		Expect(out).To(ContainSubstring("\033[32m"))
		Expect(out).To(ContainSubstring("up to date"))
	})

	// STAT-3: Line runner indicator at the top with ⏸/▶ symbols
	It("shows ⏸ indicator and config filename when no runner is active [STAT-3]", func() {
		out := lineOK(dir, "status")
		Expect(out).To(ContainSubstring("\033[90m⏸\033[0m"))
		Expect(out).To(ContainSubstring("line.yaml"))
	})

	// STAT-2: Shows "agent running" with orange colour and uptime duration
	It("shows agent running status with uptime duration while agent is active [STAT-2, STAT-7]", func() {
		slowAgent := writeSlowMockAgent(dir)
		writeConfig(dir, `agent:
  command: `+slowAgent+`
  args: ["-p"]

settings:
  watches: master

stations:
  - name: review
    prompt: "Review code"
`)
		writeFile(dir, "code.go", "package main\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add code")

		// Start run in background (slow agent sleeps 30s)
		cmd := exec.Command(binaryPath, "run")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		err := cmd.Start()
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}()

		// Wait for the station PID file (agent is running)
		Eventually(func() bool {
			return fileExists(dir, ".line/stations/review.pid")
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

		// Query status while agent is running
		out := lineOK(dir, "status")

		// Should show orange-coloured "agent running"
		Expect(out).To(ContainSubstring("\033[33m"))
		Expect(out).To(ContainSubstring("agent running"))
		// STAT-7: Should show uptime duration, not PID
		Expect(out).To(MatchRegexp(`\(\d+[smh]`))
		Expect(out).NotTo(ContainSubstring("PID"))
	})

	// STAT-3: Line runner shows as active while a run is in progress
	It("shows line as active while a run is in progress [STAT-3]", func() {
		slowAgent := writeSlowMockAgent(dir)
		writeConfig(dir, `agent:
  command: `+slowAgent+`
  args: ["-p"]

settings:
  watches: master

stations:
  - name: review
    prompt: "Review code"
`)
		writeFile(dir, "code.go", "package main\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add code")

		// Start run in background
		cmd := exec.Command(binaryPath, "run")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		err := cmd.Start()
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}()

		// Wait for PID file (line is running)
		Eventually(func() bool {
			return fileExists(dir, ".line/run.pid")
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

		out := lineOK(dir, "status")
		Expect(out).To(ContainSubstring("\033[32m▶\033[0m"))
		Expect(out).To(ContainSubstring("line.yaml"))
	})

	// STAT-5: State computed on-demand, not cached in files
	It("computes status on-demand from git state, surviving deletion of state files [STAT-5]", func() {
		agentScript := writeMockAgent(dir)
		writeConfig(dir, `agent:
  command: `+agentScript+`
  args: ["-p"]

settings:
  watches: master

stations:
  - name: review
    prompt: "Review code"
`)
		writeFile(dir, "code.go", "package main\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add code")

		lineOK(dir, "run")

		// Verify status shows "up to date"
		out := lineOK(dir, "status")
		Expect(out).To(ContainSubstring("up to date"))

		// Delete ALL cached state files
		os.RemoveAll(filepath.Join(dir, ".line"))

		// Status should STILL show "up to date" because it's computed from git
		out = lineOK(dir, "status")
		Expect(out).To(ContainSubstring("up to date"))

		// And line should show inactive (no PID file, derived on-demand)
		Expect(out).To(ContainSubstring("\033[90m⏸\033[0m"))
	})

	// STAT-6: Status symbols next to each station with column headers
	It("shows status symbols and column headers [STAT-6]", func() {
		out := lineOK(dir, "status")
		// Should show column headers
		Expect(out).To(ContainSubstring("Stations"))
		Expect(out).To(ContainSubstring("Head"))
		Expect(out).To(ContainSubstring("Status"))
		// Pending station should show ○ symbol
		Expect(out).To(ContainSubstring("○"))
	})

	// STAT-6: Shows ✓ symbol for up-to-date stations
	It("shows ✓ symbol for up-to-date stations [STAT-6]", func() {
		agentScript := writeMockAgent(dir)
		writeConfig(dir, `agent:
  command: `+agentScript+`
  args: ["-p"]

settings:
  watches: master

stations:
  - name: review
    prompt: "Review code"
`)
		writeFile(dir, "code.go", "package main\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add code")

		lineOK(dir, "run")

		out := lineOK(dir, "status")
		Expect(out).To(ContainSubstring("✓"))
		Expect(out).To(ContainSubstring("\033[32m"))
		Expect(out).To(ContainSubstring("✓"))
	})

	// STAT-4: Follow mode refreshes with updated data
	It("refreshes with updated data in follow mode [STAT-4]", func() {
		agentScript := writeMockAgent(dir)
		writeConfig(dir, `agent:
  command: `+agentScript+`
  args: ["-p"]

settings:
  watches: master

stations:
  - name: review
    prompt: "Review code"
`)

		// Start status -f capturing output
		cmd := exec.Command(binaryPath, "status", "-f")
		cmd.Dir = dir
		outBuf := &bytes.Buffer{}
		cmd.Stdout = outBuf
		cmd.Stderr = outBuf
		err := cmd.Start()
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}()

		// Let the first render happen
		time.Sleep(500 * time.Millisecond)

		// Capture initial output - should show "pending"
		initial := outBuf.String()
		Expect(initial).To(ContainSubstring("pending"))

		// Now run the pipeline so status changes
		writeFile(dir, "code.go", "package main\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add code")
		lineOK(dir, "run")

		// Wait for at least one refresh cycle (2 seconds)
		time.Sleep(3 * time.Second)

		// The output should now also contain "up to date" from a later refresh
		updated := outBuf.String()
		Expect(updated).To(ContainSubstring("up to date"))

		// Follow mode must emit erase-to-end-of-line escapes to prevent
		// stale characters when lines shrink between refreshes.
		Expect(updated).To(ContainSubstring("\033[K"))

		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	// STAT-10: Commit distance indicator column
	It("shows H on master and stations when all are at HEAD [STAT-10]", func() {
		agentScript := writeMockAgent(dir)
		writeConfig(dir, `agent:
  command: `+agentScript+`
  args: ["-p"]

settings:
  watches: master

stations:
  - name: review
    prompt: "Review code"
`)
		writeFile(dir, "code.go", "package main\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add code")

		lineOK(dir, "run")

		out := lineOK(dir, "status")
		// Station should show H+ (ahead of HEAD by agent commit)
		Expect(out).To(MatchRegexp(`review\s+H\+`))
		// Master row should show just H (no dashes, stations aren't behind)
		Expect(out).To(MatchRegexp(`master\s+H\s+[0-9a-f]`))
	})

	It("shows dashes on master and stations when HEAD is ahead [STAT-10]", func() {
		agentScript := writeMockAgent(dir)
		writeConfig(dir, `agent:
  command: `+agentScript+`
  args: ["-p"]

settings:
  watches: master

stations:
  - name: review
    prompt: "Review code"
`)
		writeFile(dir, "code.go", "package main\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add code")

		lineOK(dir, "run")

		// Add two more commits to master so HEAD is 2 ahead of station
		writeFile(dir, "extra1.go", "package main\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "extra 1")
		writeFile(dir, "extra2.go", "package main\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "extra 2")

		out := lineOK(dir, "status")
		// Master should show --H (two dashes for two commits ahead)
		Expect(out).To(MatchRegexp(`master\s+--H\s+[0-9a-f]`))
		// Station should show -- (two behind)
		Expect(out).To(MatchRegexp(`review\s+--\s+[0-9a-f]`))
	})

	It("shows no indicator for stations without branches [STAT-10]", func() {
		out := lineOK(dir, "status")
		// Master should show H
		Expect(out).To(MatchRegexp(`master\s+H\s+[0-9a-f]`))
		// Station has no branch, ref is "-", no indicator between name and ref
		Expect(out).To(MatchRegexp(`review\s+-\s+\[pending\]`))
	})

	// STAT-9: After /line-rebase, all stations should show "up to date"
	// because their work is already contained in the watched branch.
	It("shows all stations as up to date after line-rebase picks up terminal station [STAT-9]", func() {
		// Use an agent that writes to unique files per station (based on prompt)
		// to avoid merge conflicts that would trigger RUN-6 resets.
		agentScript := writeMockAgentScript(dir, "unique-agent.sh", `#!/bin/bash
PROMPT="${@: -1}"
FNAME="$(echo "$PROMPT" | tr ' ' '-').txt"
echo "agent was here" >> "$FNAME"
`)
		writeConfig(dir, `agent:
  command: `+agentScript+`
  args: ["-p"]

settings:
  watches: master

stations:
  - name: docs
    prompt: "docs"
  - name: deadcode
    prompt: "deadcode"
  - name: dry
    prompt: "dry"
`)
		writeFile(dir, "code.go", "package main\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add code")

		// First run: creates station branches (fast-forward, no merge commits)
		lineOK(dir, "run")

		// Second commit on master to trigger divergence
		writeFile(dir, "extra.go", "package main\n")
		git(dir, "add", ".")
		git(dir, "commit", "-m", "add extra")

		// Second run: stations rebase onto predecessors
		lineOK(dir, "run")

		// Simulate /line-rebase: rebase master onto terminal station
		git(dir, "rebase", "line/stn/dry")

		// All stations should show "up to date", not "pending"
		out := lineOK(dir, "status")
		Expect(out).NotTo(ContainSubstring("pending"),
			"no station should be pending after line-rebase picks up all station work")
	})
})
