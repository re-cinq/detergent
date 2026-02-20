package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/re-cinq/assembly-line/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(gateCmd)
}

var gateCmd = &cobra.Command{
	Use:   "gate",
	Short: "Run pre-commit quality gates",
	Long: `Run all configured quality gates (linters, formatters, type checkers).
Each gate's run command is executed in order. If any gate fails, execution
stops immediately and the command exits with a non-zero code.

The placeholder {staged} in a gate's run string is replaced with the
space-separated list of staged file paths.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}

		if errs := config.ValidateGates(cfg.Gates); len(errs) > 0 {
			for _, e := range errs {
				fmt.Fprintf(os.Stderr, "Error: %s\n", e)
			}
			return fmt.Errorf("%d gate validation error(s)", len(errs))
		}

		if len(cfg.Gates) == 0 {
			fmt.Println("No gates configured.")
			return nil
		}

		repoDir, err := resolveRepo(configPath)
		if err != nil {
			return err
		}

		staged, err := stagedFiles(repoDir)
		if err != nil {
			return err
		}

		for _, g := range cfg.Gates {
			fmt.Printf("--- %s ---\n", g.Name)

			runStr := strings.ReplaceAll(g.Run, "{staged}", staged)
			c := exec.Command("sh", "-c", runStr)
			c.Dir = repoDir
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr

			if err := c.Run(); err != nil {
				return fmt.Errorf("gate %q failed", g.Name)
			}
		}

		return nil
	},
}

// stagedFiles returns a space-separated list of staged file paths.
func stagedFiles(repoDir string) (string, error) {
	cmd := exec.Command("git", "diff", "--cached", "--name-only")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("getting staged files: %w", err)
	}
	files := strings.TrimSpace(string(out))
	// Replace newlines with spaces for shell substitution
	return strings.ReplaceAll(files, "\n", " "), nil
}
