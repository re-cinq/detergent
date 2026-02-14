package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(validateCmd)
}

var validateCmd = &cobra.Command{
	Use:   "validate <config-file>",
	Short: "Validate a detergent configuration file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := loadAndValidateConfig(args[0]); err != nil {
			return err
		}

		fmt.Println("Configuration is valid.")
		return nil
	},
}
