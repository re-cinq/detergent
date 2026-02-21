package cli

import (
	"fmt"

	"github.com/re-cinq/assembly-line/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(vizCmd)
}

var vizCmd = &cobra.Command{
	Use:   "viz",
	Short: "Visualize the station line",
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
	downstream := cfg.BuildDownstreamMap()

	nodes := make(map[string]*vizNode)
	for _, c := range cfg.Stations {
		nodes[c.Name] = &vizNode{
			watches:    c.Watches,
			downstream: downstream[c.Name],
		}
	}

	roots := cfg.FindRoots()

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
