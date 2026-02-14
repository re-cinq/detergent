package cli

import (
	"encoding/json"
	"fmt"

	"github.com/fission-ai/detergent/internal/config"
	"github.com/fission-ai/detergent/internal/engine"
	gitops "github.com/fission-ai/detergent/internal/git"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(statuslineDataCmd)
}

var statuslineDataCmd = &cobra.Command{
	Use:   "statusline-data <config-file>",
	Short: "Output JSON status data for all concerns (for statusline rendering)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadAndValidateConfig(args[0])
		if err != nil {
			return err
		}

		repoDir, err := resolveRepo(args[0])
		if err != nil {
			return err
		}

		return outputStatuslineData(cfg, repoDir)
	},
}

// StatuslineOutput is the top-level JSON blob for statusline rendering.
type StatuslineOutput struct {
	Concerns     []ConcernData `json:"concerns"`
	Roots        []string      `json:"roots"`
	Graph        []GraphEdge   `json:"graph"`
	BranchPrefix string        `json:"branch_prefix"`
}

// ConcernData represents one concern's status for statusline rendering.
type ConcernData struct {
	Name        string `json:"name"`
	Watches     string `json:"watches"`
	State       string `json:"state"`
	LastResult  string `json:"last_result,omitempty"`
	HeadCommit  string `json:"head_commit,omitempty"`
	LastSeen    string `json:"last_seen,omitempty"`
	Error       string `json:"error,omitempty"`
	BehindHead  bool   `json:"behind_head"`
}

// GraphEdge represents a dependency: Child watches Parent.
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// gatherStatuslineData collects status data for all concerns without serializing.
func gatherStatuslineData(cfg *config.Config, repoDir string) StatuslineOutput {
	repo := gitops.NewRepo(repoDir)

	nameSet := make(map[string]bool)
	for _, c := range cfg.Concerns {
		nameSet[c.Name] = true
	}

	concerns := make([]ConcernData, 0)
	roots := make([]string, 0)
	graph := make([]GraphEdge, 0)

	for _, c := range cfg.Concerns {
		// Build graph edges
		if nameSet[c.Watches] {
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
				cd.State = "failed"
				cd.Error = fmt.Sprintf("process %d no longer running", status.PID)
			}
		} else {
			cd.State = "unknown"
		}

		// Get HEAD of watched branch to determine if behind
		watchedBranch := c.Watches
		if nameSet[c.Watches] {
			watchedBranch = cfg.Settings.BranchPrefix + c.Watches
		}
		if head, err := repo.HeadCommit(watchedBranch); err == nil {
			cd.HeadCommit = head
			if cd.LastSeen != "" && cd.LastSeen != head {
				cd.BehindHead = true
			}
		}

		// Normalize: idle + caught up + no last_result → noop
		if cd.State == "idle" && cd.LastResult == "" && cd.LastSeen != "" && !cd.BehindHead {
			cd.LastResult = "noop"
		}

		// Normalize: idle + behind HEAD + previously ran → pending
		if cd.State == "idle" && cd.BehindHead && cd.LastSeen != "" {
			cd.State = "pending"
		}

		concerns = append(concerns, cd)
	}

	return StatuslineOutput{
		Concerns:     concerns,
		Roots:        roots,
		Graph:        graph,
		BranchPrefix: cfg.Settings.BranchPrefix,
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
