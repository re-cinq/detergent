package cli

import (
	"fmt"

	"github.com/fission-ai/detergent/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(vizCmd)
}

var vizCmd = &cobra.Command{
	Use:   "viz",
	Short: "Visualize the concern graph",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadAndValidateConfig(configPath)
		if err != nil {
			return err
		}

		printGraph(cfg)
		return nil
	},
}

type vizNode struct {
	watches    string
	downstream []string
}

func printGraph(cfg *config.Config) {
	nameSet := make(map[string]bool)
	for _, c := range cfg.Concerns {
		nameSet[c.Name] = true
	}

	nodes := make(map[string]*vizNode)
	for _, c := range cfg.Concerns {
		nodes[c.Name] = &vizNode{watches: c.Watches}
	}

	// Build downstream edges: if B watches A, then A -> B
	for _, c := range cfg.Concerns {
		if nameSet[c.Watches] {
			nodes[c.Watches].downstream = append(nodes[c.Watches].downstream, c.Name)
		}
	}

	// Roots watch external branches (not other concerns)
	var roots []string
	for _, c := range cfg.Concerns {
		if !nameSet[c.Watches] {
			roots = append(roots, c.Name)
		}
	}

	for _, root := range roots {
		fmt.Printf("[%s]\n", nodes[root].watches)
		printBranch(nodes, root, "", true)
	}
}

func printBranch(nodes map[string]*vizNode, name string, prefix string, isLast bool) {
	connector := "├── "
	if isLast {
		connector = "└── "
	}

	fmt.Printf("%s%s%s\n", prefix, connector, name)

	childPrefix := prefix
	if isLast {
		childPrefix += "    "
	} else {
		childPrefix += "│   "
	}

	n := nodes[name]
	for i, child := range n.downstream {
		printBranch(nodes, child, childPrefix, i == len(n.downstream)-1)
	}
}
