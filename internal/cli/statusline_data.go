package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/re-cinq/assembly-line/internal/config"
	"github.com/re-cinq/assembly-line/internal/engine"
	gitops "github.com/re-cinq/assembly-line/internal/git"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(statuslineDataCmd)
}

var statuslineDataCmd = &cobra.Command{
	Use:    "statusline-data",
	Short:  "Output JSON status data for all stations (for statusline rendering)",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, repoDir, err := loadConfigAndRepo(configPath)
		if err != nil {
			return err
		}

		return outputStatuslineData(cfg, repoDir)
	},
}

// StatuslineOutput is the top-level JSON blob for statusline rendering.
type StatuslineOutput struct {
	SourceBranch       string        `json:"source_branch"`
	SourceCommit       string        `json:"source_commit,omitempty"`
	Dirty              bool          `json:"dirty"`
	RunnerAlive        bool          `json:"runner_alive"`
	RunnerPID          int           `json:"runner_pid,omitempty"`
	RunnerSince        string        `json:"runner_since,omitempty"`
	Stations           []StationData `json:"stations"`
	Roots              []string      `json:"roots"`
	Graph              []GraphEdge   `json:"graph"`
	HasUnpickedCommits bool          `json:"has_unpicked_commits"`
}

// StationData represents one station's status for statusline rendering.
type StationData struct {
	Name       string `json:"name"`
	Watches    string `json:"watches"`
	State      string `json:"state"`
	LastResult string `json:"last_result,omitempty"`
	HeadCommit string `json:"head_commit,omitempty"`
	Error      string `json:"error,omitempty"`
	BehindHead bool   `json:"behind_head"`
}

// GraphEdge represents a dependency: Child watches Parent.
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// gatherStatuslineData collects status data for all stations without serializing.

func gatherStatuslineData(cfg *config.Config, repoDir string) StatuslineOutput {
	repo := gitops.NewRepo(repoDir)

	stations := make([]StationData, 0)
	roots := cfg.FindRoots()
	graph := make([]GraphEdge, 0)

	for _, c := range cfg.Stations {
		// Build graph edges
		if cfg.HasStation(c.Watches) {
			graph = append(graph, GraphEdge{From: c.Watches, To: c.Name})
		}

		// Read status file
		status, _ := engine.ReadStatus(repoDir, c.Name)

		cd := StationData{
			Name:    c.Name,
			Watches: c.Watches,
		}

		if status != nil {
			cd.State = status.State
			cd.LastResult = status.LastResult
			cd.Error = status.Error

			// Detect stale active states (process died)
			if engine.IsActiveState(cd.State) && !engine.IsProcessAlive(status.PID) {
				cd.State = engine.StateFailed
				cd.Error = fmt.Sprintf("process %d no longer running", status.PID)
			}
		} else {
			cd.State = "unknown"
		}

		// Read last-seen from the canonical state file
		lastSeen, _ := engine.LastSeen(repoDir, c.Name)

		// Get HEAD of watched branch to determine if behind
		watchedBranch := engine.ResolveWatchedBranch(cfg, c)
		if head, err := repo.HeadCommit(watchedBranch); err == nil {
			cd.HeadCommit = head
			if lastSeen != "" && lastSeen != head {
				cd.BehindHead = true
			}
		}

		// Normalize: idle + caught up + no last_result → noop
		if cd.State == engine.StateIdle && cd.LastResult == "" && lastSeen != "" && !cd.BehindHead {
			cd.LastResult = engine.ResultNoop
		}

		// Normalize: idle + behind HEAD + previously ran → pending
		if cd.State == engine.StateIdle && cd.BehindHead && lastSeen != "" {
			cd.State = "pending"
		}

		stations = append(stations, cd)
	}

	// Determine if the terminal station branch has commits ahead of the root watched branch.
	hasUnpicked := false
	downstream := make(map[string]bool)
	for _, e := range graph {
		downstream[e.From] = true
	}
	var terminals []string
	for _, c := range stations {
		if !downstream[c.Name] {
			terminals = append(terminals, c.Name)
		}
	}
	if len(terminals) == 1 {
		terminalBranch := cfg.Settings.BranchPrefix + terminals[0]
		rootWatched := cfg.Settings.Watches
		if repo.BranchExists(terminalBranch) {
			if commits, err := repo.CommitsBetween(rootWatched, terminalBranch); err == nil && len(commits) > 0 {
				hasUnpicked = true
			}
		}
	}

	// Source branch info
	sourceBranch := cfg.Settings.Watches
	sourceCommit := ""
	dirty := false
	if head, err := repo.HeadCommit(sourceBranch); err == nil {
		sourceCommit = head
	}
	if d, err := repo.HasChanges(); err == nil {
		dirty = d
	}

	// Runner status
	runnerAlive := engine.IsRunnerAlive(repoDir)
	runnerPID := 0
	runnerSince := ""
	if runnerAlive {
		runnerPID = engine.ReadPID(repoDir)
		if info, err := os.Stat(engine.PIDPath(repoDir)); err == nil {
			runnerSince = info.ModTime().UTC().Format("2006-01-02T15:04:05Z")
		}
	}

	return StatuslineOutput{
		SourceBranch:       sourceBranch,
		SourceCommit:       sourceCommit,
		Dirty:              dirty,
		RunnerAlive:        runnerAlive,
		RunnerPID:          runnerPID,
		RunnerSince:        runnerSince,
		Stations:           stations,
		Roots:              roots,
		Graph:              graph,
		HasUnpickedCommits: hasUnpicked,
	}
}

func outputStatuslineData(cfg *config.Config, repoDir string) error {
	output := gatherStatuslineData(cfg, repoDir)
	data, err := json.Marshal(output)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
