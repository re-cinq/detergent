package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

const explainText = `line — Assembly Line for Git Commits

PURPOSE
  line automates pre-commit checks (gates) and post-commit agent tasks
  (stations) using Git hooks. Gates run deterministic tools like linters before
  a commit is accepted. Stations run an agent (e.g. Claude Code) against each
  new commit in an ordered pipeline, each on its own Git branch.

  Rather than relying on an agent to remember to run checks, line hard-codes
  a sequence of prompts that run automatically on every commit.

COMMANDS
  init        Install Git hooks (pre-commit for gates, post-commit for run),
              the /line-rebase and /line-preview skills, and configure
              Claude Code's statusline.
              Adds .gitignore entries for temporary files introduced by line.
              Preserves any existing pre-commit hooks. Safe to re-run —
              converges state.
  remove      Undo everything that init installs, creates, or configures.
              Removes assembly-line blocks from pre-commit and post-commit
              hooks (preserving other content), removes the /line-rebase and
              /line-preview skill directories, removes the statusLine key
              from .claude/settings.json, and removes the assembly-line block
              from .gitignore. Safe to run even when line was never initialized
              (no-op).
  run         Execute the station pipeline (called by the post-commit hook).
              Stations run in sequence, each in an ephemeral Git worktree.
  gate        Run all gates (called by the pre-commit hook). Non-zero exit
              from any gate blocks the commit.
  status      Show station status. Header: ⏸ (grey) for inactive or ▶ (green)
              for active, followed by the config file name. Output includes
              headings. Stations listed starting with the watched branch; each
              shows a shortref of HEAD and a dirty-directory indicator, with
              per-station symbols: ✓ up-to-date — the only commits between
              the station and the watched branch HEAD are skip-marker commits
              (green); ● agent running (orange, with uptime, e.g. 52s/5m 32s);
              ○ pending (yellow); ✗ failed (red). Use -f to refresh every
              2 seconds, flicker-free with a hidden cursor. Status is
              computed on-demand, not cached.
  statusline  One-line status for Claude Code's statusline integration.
              Uses ▶/⏸ symbols matching line status. Prompts to run
              /line-rebase when terminal station has unmerged commits.
              No external dependencies.
  schema      Output the YAML configuration schema to stdout.
  validate    Validate line.yaml and print specific errors, or "valid".
  explain     Print this reference (what you are reading now).

  Skill: /line-rebase
    Safely rebase changes from the terminal station branch back onto the
    watched branch. Stashes work, rebases, unstashes. No work is lost.
    Commits picked onto the watched branch are marked to avoid re-triggering
    the line.

  Skill: /line-preview
    Read-only summary of unpicked changes: what each station actually
    changed (content diffs), not commit history. All derived from Git
    on-demand with no state files.

CONFIG FORMAT (line.yaml)
  All commands assume the config is at line.yaml in the current directory.
  Commands that reference config accept -p/--path to specify a different path.

  agent:
    command: claude                              # default agent executable
    args: ["--dangerously-skip-permissions", "-p"]  # default agent arguments

  settings:
    watches: main                                # Git branch to watch (required)

  gates:
    - name: lint                                 # gate name (required)
      run: "golangci-lint run ./..."             # shell command (required)

  stations:
    - name: review                               # unique name → branch line/stn/review
      prompt: "Review the code for issues."      # prompt text
    - name: test
      command: custom-agent                      # overrides agent.command
      args: ["--flag", "-p"]                     # overrides agent.args
      prompt: "Run all tests, fix failures."

CONFIG SEMANTICS
  - settings.watches is required. All other top-level keys are optional.
  - Each station needs a resolvable command: either station.command or
    agent.command must be set. station.command takes priority.
  - Station args follow the same inheritance: station.args overrides agent.args.
  - The prompt is appended as the final argument to the resolved command+args.
  - Station names must be unique — each maps to a Git branch (line/stn/<name>).
  - Gates run in order; any failure blocks the commit.
  - Stations run in order; a failed station blocks subsequent stations.

CONSTRAINTS
  - line prepends a preamble prompt to each station's configured prompt
    instructing the agent not to commit — line handles committing itself.
  - Each station operates only on its own branch (line/stn/<name>); stations
    must not operate on any other branches.
  - Stations must not re-trigger line run.
  - Stations run in isolated ephemeral Git worktrees under the system temp dir.
  - Commits containing [skip ci], [ci skip], [skip line], or [line skip] in
    the message do not trigger the line.
  - Changes to files listed in .lineignore (gitignore syntax) are ignored.
  - If a new commit arrives while the line is running, agents are stopped,
    existing station-branch commits are preserved, and the line restarts from
    the beginning with the latest commit.
  - Stations 'just work' — if Git state is bad they catch up to the watched
    branch and resume from there.
  - Line runs are independent of rebases on the watched branch.
  - Stations rebase onto their predecessor (not merge) to keep history linear.`

var explainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Print agent-friendly reference for assembly-line",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(explainText)
	},
}

func init() {
	rootCmd.AddCommand(explainCmd)
}
