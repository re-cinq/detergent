package cli

import (
	"encoding/json"
	"fmt"

	"github.com/re-cinq/detergent/internal/config"
	"github.com/re-cinq/detergent/internal/engine"
	gitops "github.com/re-cinq/detergent/internal/git"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(statuslineDataCmd)
}

var statuslineDataCmd = &cobra.Command{
	Use:    "statusline-data",
	Short:  "Output JSON status data for all concerns (for statusline rendering)",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadAndValidateConfig(configPath)
		if err != nil {
			return err
		}

		repoDir, err := resolveRepo(configPath)
		if err != nil {
			return err
		}

		return outputStatuslineData(cfg, repoDir)
	},
}

// StatuslineOutput is the top-level JSON blob for statusline rendering.
type StatuslineOutput struct {
	Concerns           []ConcernData `json:"concerns"`
	Roots              []string      `json:"roots"`
	Graph              []GraphEdge   `json:"graph"`
	HasUnpickedCommits bool          `json:"has_unpicked_commits"`
}

// ConcernData represents one concern's status for statusline rendering.
type ConcernData struct {
	Name       string `json:"name"`
	Watches    string `json:"watches"`
	State      string `json:"state"`
	LastResult string `json:"last_result,omitempty"`
	HeadCommit string `json:"head_commit,omitempty"`
	LastSeen   string `json:"last_seen,omitempty"`
	Error      string `json:"error,omitempty"`
	BehindHead bool   `json:"behind_head"`
}

// GraphEdge represents a dependency: Child watches Parent.
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// gatherStatuslineData collects status data for all concerns without serializing.

func gatherStatuslineData(cfg *config.Config, repoDir string) StatuslineOutput {
	repo := gitops.NewRepo(repoDir)

	concerns := make([]ConcernData, 0)
	roots := make([]string, 0)
	graph := make([]GraphEdge, 0)

	for _, c := range cfg.Concerns {
		// Build graph edges
		if cfg.HasConcern(c.Watches) {
			graph = append(graph, GraphEdge{From: c.Watches, To: c.Name})
		} else {
			roots = append(roots, c.Name)
		}

		// Read status file
		status, _ := engine.ReadStatus(repoDir, c.Name)

		cd := ConcernData{
			Name:    c.Name,
			Watches: c.Watches,
		}

		if status != nil {
			cd.State = status.State
			cd.LastResult = status.LastResult
			cd.LastSeen = status.LastSeen
			cd.Error = status.Error

			// Detect stale active states (process died)
			if engine.IsActiveState(cd.State) && !engine.IsProcessAlive(status.PID) {
				cd.State = engine.StateFailed
				cd.Error = fmt.Sprintf("process %d no longer running", status.PID)
			}
		} else {
			cd.State = "unknown"
		}

		// Get HEAD of watched branch to determine if behind
		watchedBranch := engine.ResolveWatchedBranch(cfg, c)
		if head, err := repo.HeadCommit(watchedBranch); err == nil {
			cd.HeadCommit = head
			if cd.LastSeen != "" && cd.LastSeen != head {
				cd.BehindHead = true
			}
		}

		// Normalize: idle + caught up + no last_result → noop
		if cd.State == engine.StateIdle && cd.LastResult == "" && cd.LastSeen != "" && !cd.BehindHead {
			cd.LastResult = engine.ResultNoop
		}

		// Normalize: idle + behind HEAD + previously ran → pending
		if cd.State == engine.StateIdle && cd.BehindHead && cd.LastSeen != "" {
			cd.State = "pending"
		}

		concerns = append(concerns, cd)
	}

	// Determine if the terminal concern branch has commits ahead of the root watched branch.
	hasUnpicked := false
	downstream := make(map[string]bool)
	for _, e := range graph {
		downstream[e.From] = true
	}
	var terminals []string
	for _, c := range concerns {
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

	return StatuslineOutput{
		Concerns:           concerns,
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
