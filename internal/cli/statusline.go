package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fission-ai/detergent/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(statuslineCmd)
}

var statuslineCmd = &cobra.Command{
	Use:   "statusline",
	Short: "Render concern graph for Claude Code statusline (reads JSON from stdin)",
	RunE: func(cmd *cobra.Command, args []string) error {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		dir := resolveProjectDir(input)
		if dir == "" {
			return nil // silent exit
		}

		configPath := findDetergentConfig(dir)
		if configPath == "" {
			return nil // not a detergent project
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

// findDetergentConfig walks up from dir looking for detergent.yaml or detergent.yml.
func findDetergentConfig(dir string) string {
	for {
		for _, name := range []string{"detergent.yaml", "detergent.yml"} {
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// ANSI escape codes
const (
	ansiGreen  = "\033[32m"
	ansiCyan   = "\033[36m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiDim    = "\033[2m"
	ansiReset  = "\033[0m"
)

func statusSymbol(state, lastResult string) string {
	switch state {
	case "running":
		return "⟳"
	case "failed":
		return "✗"
	case "skipped":
		return "⊘"
	case "idle":
		switch lastResult {
		case "modified":
			return "*"
		case "noop":
			return "✓"
		}
		return "·"
	case "unknown":
		return "·"
	default:
		return "◯"
	}
}

func statusColor(state, lastResult string) string {
	switch state {
	case "running":
		return ansiYellow
	case "failed":
		return ansiRed
	case "skipped":
		return ansiDim
	case "idle":
		switch lastResult {
		case "modified":
			return ansiCyan
		case "noop":
			return ansiGreen
		}
		return ansiDim
	case "unknown":
		return ansiDim
	default:
		return ansiReset
	}
}

func renderConcern(name string, concerns map[string]ConcernData) string {
	c := concerns[name]
	sym := statusSymbol(c.State, c.LastResult)
	clr := statusColor(c.State, c.LastResult)
	return fmt.Sprintf("%s%s %s%s", clr, name, sym, ansiReset)
}

// buildChain follows single-child edges from name into a linear chain.
func buildChain(name string, downstream map[string][]string) []string {
	chain := []string{name}
	for {
		children := downstream[chain[len(chain)-1]]
		if len(children) != 1 {
			break
		}
		chain = append(chain, children[0])
	}
	return chain
}

// collectBranches collects all fork arms rooted at name via DFS.
func collectBranches(name string, downstream map[string][]string) [][]string {
	chain := buildChain(name, downstream)
	last := chain[len(chain)-1]
	result := [][]string{chain}
	children := downstream[last]
	if len(children) > 1 {
		for _, child := range children {
			result = append(result, collectBranches(child, downstream)...)
		}
	}
	return result
}

func renderChain(chain []string, concerns map[string]ConcernData) string {
	parts := make([]string, len(chain))
	for i, name := range chain {
		parts[i] = renderConcern(name, concerns)
	}
	return strings.Join(parts, " ── ")
}

// renderGraph produces the full ANSI-colored graph string from statusline data.
func renderGraph(data StatuslineOutput) string {
	if len(data.Concerns) == 0 {
		return ""
	}

	concerns := make(map[string]ConcernData)
	for _, c := range data.Concerns {
		concerns[c.Name] = c
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
	for _, c := range data.Concerns {
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
			sb.WriteString(fmt.Sprintf("%s ─── %s", branch, renderChain(arms[0], concerns)))
		} else {
			sb.WriteString(fmt.Sprintf("%s ─┬─ %s", branch, renderChain(arms[0], concerns)))
			padding := strings.Repeat(" ", len(branch)+2)
			for i, arm := range arms[1:] {
				connector := "├"
				if i == len(arms)-2 { // last arm
					connector = "└"
				}
				sb.WriteString(fmt.Sprintf("\n%s%s─ %s", padding, connector, renderChain(arm, concerns)))
			}
		}

		if bi < len(branchOrder)-1 {
			sb.WriteString("\n")
		}
	}

	// Check if the chain is complete with results ready to rebase
	if hint := rebaseHint(data, concerns, downstream); hint != "" {
		sb.WriteString("\n")
		sb.WriteString(hint)
	}

	return sb.String()
}

// rebaseHint returns a prompt to use /rebase if the concern chain is complete
// with modifications ready to land. Returns "" if not applicable.
func rebaseHint(data StatuslineOutput, concerns map[string]ConcernData, downstream map[string][]string) string {
	if len(concerns) == 0 {
		return ""
	}

	// Find terminal concerns (not in any downstream edge's From)
	hasChildren := make(map[string]bool)
	for from := range downstream {
		hasChildren[from] = true
	}
	var terminals []string
	for name := range concerns {
		if !hasChildren[name] {
			terminals = append(terminals, name)
		}
	}

	// Only support linear chains (single terminal)
	if len(terminals) != 1 {
		return ""
	}
	terminal := terminals[0]

	// All concerns must be idle
	for _, c := range concerns {
		if c.State == "running" || c.State == "failed" {
			return ""
		}
	}

	// Terminal must have produced modifications
	tc := concerns[terminal]
	if tc.LastResult != "modified" {
		return ""
	}

	branch := data.BranchPrefix + terminal
	return fmt.Sprintf("%suse /rebase %s to pick up latest changes%s", ansiDim, branch, ansiReset)
}
