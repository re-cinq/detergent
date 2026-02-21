package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/re-cinq/assembly-line/internal/config"
	"github.com/re-cinq/assembly-line/internal/engine"
	"github.com/spf13/cobra"
)

func init() {
	statuslineCmd.Hidden = true
	rootCmd.AddCommand(statuslineCmd)
}

var statuslineCmd = &cobra.Command{
	Use:   "statusline",
	Short: "Render station line for Claude Code statusline (reads JSON from stdin)",
	RunE: func(cmd *cobra.Command, args []string) error {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		dir := resolveProjectDir(input)
		if dir == "" {
			return nil // silent exit
		}

		configPath := findLineConfig(dir)
		if configPath == "" {
			return nil // not a line project
		}

		cfg, err := config.Load(configPath)
		if err != nil {
			return nil // silent exit on bad config
		}
		if errs := config.Validate(cfg); len(errs) > 0 {
			return nil // silent exit on invalid config
		}

		repoDir := findGitRoot(filepath.Dir(configPath))
		if repoDir == "" {
			return nil
		}

		data := gatherStatuslineData(cfg, repoDir)
		rendered := renderGraph(data)
		if rendered != "" {
			fmt.Print(rendered)
		}
		return nil
	},
}

// claudeCodeInput represents the JSON object Claude Code passes on stdin.
type claudeCodeInput struct {
	CWD       string `json:"cwd"`
	Workspace *struct {
		ProjectDir string `json:"project_dir"`
	} `json:"workspace"`
}

// resolveProjectDir extracts the project directory from Claude Code's stdin JSON.
func resolveProjectDir(input []byte) string {
	var ci claudeCodeInput
	if err := json.Unmarshal(input, &ci); err != nil {
		return ""
	}
	if ci.Workspace != nil && ci.Workspace.ProjectDir != "" {
		return ci.Workspace.ProjectDir
	}
	return ci.CWD
}

// findLineConfig walks up from dir looking for line.yaml or line.yml.
func findLineConfig(dir string) string {
	return findFileUp(dir, []string{"line.yaml", "line.yml"})
}

func renderStation(name string, stations map[string]StationData) string {
	c := stations[name]
	sym, clr := stateDisplay(c.State, c.LastResult)
	return fmt.Sprintf("%s%s %s%s", clr, name, sym, ansiReset)
}

// buildLine follows single-child edges from name into a linear line.
func buildLine(name string, downstream map[string][]string) []string {
	line := []string{name}
	for {
		children := downstream[line[len(line)-1]]
		if len(children) != 1 {
			break
		}
		line = append(line, children[0])
	}
	return line
}

// collectBranches collects all fork arms rooted at name via DFS.
func collectBranches(name string, downstream map[string][]string) [][]string {
	line := buildLine(name, downstream)
	last := line[len(line)-1]
	result := [][]string{line}
	children := downstream[last]
	if len(children) > 1 {
		for _, child := range children {
			result = append(result, collectBranches(child, downstream)...)
		}
	}
	return result
}

func renderLine(line []string, stations map[string]StationData) string {
	parts := make([]string, len(line))
	for i, name := range line {
		parts[i] = renderStation(name, stations)
	}
	return strings.Join(parts, " ── ")
}

// renderGraph produces the full ANSI-colored graph string from statusline data.
func renderGraph(data StatuslineOutput) string {
	if len(data.Stations) == 0 {
		return ""
	}

	stations := make(map[string]StationData)
	for _, c := range data.Stations {
		stations[c.Name] = c
	}

	// Build downstream adjacency: parent -> [children]
	downstream := make(map[string][]string)
	for _, edge := range data.Graph {
		downstream[edge.From] = append(downstream[edge.From], edge.To)
	}

	// Group roots by their watched branch
	branchRoots := make(map[string][]string)
	// Preserve branch order from config
	var branchOrder []string
	rootSet := make(map[string]bool)
	for _, r := range data.Roots {
		rootSet[r] = true
	}
	for _, c := range data.Stations {
		if rootSet[c.Name] {
			if _, seen := branchRoots[c.Watches]; !seen {
				branchOrder = append(branchOrder, c.Watches)
			}
			branchRoots[c.Watches] = append(branchRoots[c.Watches], c.Name)
		}
	}

	var sb strings.Builder
	for bi, branch := range branchOrder {
		rootNames := branchRoots[branch]

		// Collect all fork arms
		var arms [][]string
		for _, rn := range rootNames {
			arms = append(arms, collectBranches(rn, downstream)...)
		}

		if len(arms) == 1 {
			sb.WriteString(fmt.Sprintf("%s ─── %s", branch, renderLine(arms[0], stations)))
		} else {
			sb.WriteString(fmt.Sprintf("%s ─┬─ %s", branch, renderLine(arms[0], stations)))
			padding := strings.Repeat(" ", len(branch)+2)
			for i, arm := range arms[1:] {
				connector := "├"
				if i == len(arms)-2 { // last arm
					connector = "└"
				}
				sb.WriteString(fmt.Sprintf("\n%s%s─ %s", padding, connector, renderLine(arm, stations)))
			}
		}

		if bi < len(branchOrder)-1 {
			sb.WriteString("\n")
		}
	}

	// Check if the line is complete with results ready to rebase
	if hint := rebaseHint(data, stations, downstream); hint != "" {
		sb.WriteString("\n")
		sb.WriteString(hint)
	}

	return sb.String()
}

// rebaseHint returns a prompt to use /line-rebase if the station line is complete
// with modifications ready to land. Returns "" if not applicable.
func rebaseHint(data StatuslineOutput, stations map[string]StationData, downstream map[string][]string) string {
	if len(stations) == 0 {
		return ""
	}

	// Find terminal stations (not in any downstream edge's From)
	hasChildren := make(map[string]bool)
	for from := range downstream {
		hasChildren[from] = true
	}
	var terminals []string
	for name := range stations {
		if !hasChildren[name] {
			terminals = append(terminals, name)
		}
	}

	// Only support linear lines (single terminal)
	if len(terminals) != 1 {
		return ""
	}

	// All stations must be idle
	for _, c := range stations {
		switch c.State {
		case engine.StateChangeDetected, engine.StateAgentRunning, engine.StateCommitting, engine.StateFailed, "pending":
			return ""
		}
	}

	// The terminal station branch must have commits ahead of the root watched branch
	if !data.HasUnpickedCommits {
		return ""
	}

	return fmt.Sprintf("\033[1;33m⚠ use /line-rebase to pick up latest changes%s", ansiReset)
}
