package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fission-ai/detergent/internal/config"
)

// loadAndValidateConfig loads a config file and validates it, printing errors to stderr.
func loadAndValidateConfig(path string) (*config.Config, error) {
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return nil, err
	}

	errs := config.Validate(cfg)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "Error: %s\n", e)
		}
		return nil, fmt.Errorf("%d validation error(s)", len(errs))
	}

	return cfg, nil
}

// resolveRepo finds the git repository root from a config file path.
func resolveRepo(configArg string) (string, error) {
	configPath, err := filepath.Abs(configArg)
	if err != nil {
		return "", err
	}
	repoDir := findGitRoot(filepath.Dir(configPath))
	if repoDir == "" {
		return "", fmt.Errorf("could not find git repository root")
	}
	return repoDir, nil
}

// findGitRoot walks up from dir looking for a .git directory.
func findGitRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
