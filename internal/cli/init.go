package cli

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/re-cinq/detergent/internal/assets"
	"github.com/re-cinq/detergent/internal/fileutil"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Initialize detergent skills and statusline in a repository",
	Long: `Initialize detergent Claude Code skills and statusline configuration
in the target repository (defaults to current directory).

This command:
  - Copies skill files into .claude/skills/
  - Configures the Claude Code statusline in .claude/settings.local.json`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}

		absDir, err := filepath.Abs(dir)
		if err != nil {
			return fmt.Errorf("resolving path: %w", err)
		}

		// Verify it's a git repo
		if _, err := os.Stat(filepath.Join(absDir, ".git")); err != nil {
			return fmt.Errorf("%s is not a git repository (no .git directory)", absDir)
		}

		installed, err := initSkills(absDir)
		if err != nil {
			return fmt.Errorf("installing skills: %w", err)
		}
		for _, path := range installed {
			fmt.Printf("  skill  %s\n", path)
		}

		if err := initStatusline(absDir); err != nil {
			return fmt.Errorf("configuring statusline: %w", err)
		}
		fmt.Println("  config .claude/settings.local.json (statusline)")

		fmt.Println("\nDone.")
		return nil
	},
}

// initSkills copies all embedded skill files into .claude/skills/.
func initSkills(repoDir string) ([]string, error) {
	var installed []string

	err := fs.WalkDir(assets.Skills, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Target path: .claude/<path> (skills/detergent-rebase/SKILL.md -> .claude/skills/detergent-rebase/SKILL.md)
		target := fileutil.ClaudeSubpath(repoDir, path)

		if d.IsDir() {
			return fileutil.EnsureDir(target)
		}

		data, err := assets.Skills.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading embedded %s: %w", path, err)
		}

		if err := os.WriteFile(target, data, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", target, err)
		}

		rel, err := filepath.Rel(repoDir, target)
		if err != nil {
			return fmt.Errorf("computing relative path for %s: %w", target, err)
		}
		installed = append(installed, rel)
		return nil
	})

	return installed, err
}

// initStatusline adds or updates the statusline config in .claude/settings.local.json.
func initStatusline(repoDir string) error {
	detergentBin, err := os.Executable()
	if err != nil {
		// Fall back to expecting it in PATH
		detergentBin = "detergent"
	}

	settingsPath := fileutil.ClaudeSubpath(repoDir, "settings.local.json")

	// Ensure .claude/ exists
	if err := fileutil.EnsureDir(fileutil.ClaudeDir(repoDir)); err != nil {
		return err
	}

	// Load existing settings or start fresh
	settings := make(map[string]interface{})
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parsing existing %s: %w", settingsPath, err)
		}
	}

	// Set statusline
	settings["statusLine"] = map[string]string{
		"command": detergentBin + " statusline",
		"type":    "command",
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}

	return os.WriteFile(settingsPath, append(out, '\n'), 0o644)
}
