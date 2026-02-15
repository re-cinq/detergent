package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/re-cinq/detergent/internal/config"
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
	return walkUpUntil(dir, func(d string) bool {
		_, err := os.Stat(filepath.Join(d, ".git"))
		return err == nil
	})
}

// walkUpUntil walks up the directory tree from dir, calling check on each directory.
// Returns the first directory where check returns true, or "" if none found.
func walkUpUntil(dir string, check func(string) bool) string {
	for {
		if check(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// findFileUp walks up from dir looking for any of the given filenames.
// Returns the full path to the first file found, or "" if none found.
func findFileUp(dir string, filenames []string) string {
	foundDir := walkUpUntil(dir, func(d string) bool {
		for _, name := range filenames {
			if _, err := os.Stat(filepath.Join(d, name)); err == nil {
				return true
			}
		}
		return false
	})
	if foundDir == "" {
		return ""
	}
	// Return the full path to the first file that exists
	for _, name := range filenames {
		p := filepath.Join(foundDir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
