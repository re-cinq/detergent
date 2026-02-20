package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags
var Version = "dev"

var configPath string

var rootCmd = &cobra.Command{
	Use:   "line",
	Short: "Orchestrate coding agents in a concern-based pipeline",
	Long: `Assembly Line is a daemon that orchestrates coding agents through a chain
of concerns. Each concern focuses on a single quality aspect (security,
style, documentation, etc.) and processes code changes in sequence.

Changes flow through the concern chain with Git providing the audit trail
and intent preservation between agents.`,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configPath, "path", "p", "line.yaml", "Path to line config file")
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("line %s\n", Version)
	},
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}
