package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// StateDir returns the state directory path for a repo.
func StateDir(repoDir string) string {
	return filepath.Join(repoDir, ".detergent", "state")
}

// LastSeen returns the last-seen commit hash for a concern, or "" if none.
func LastSeen(repoDir, concernName string) (string, error) {
	path := filepath.Join(StateDir(repoDir), concernName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading last-seen for %s: %w", concernName, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// SetLastSeen records the last-seen commit hash for a concern.
func SetLastSeen(repoDir, concernName, hash string) error {
	dir := StateDir(repoDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, concernName), []byte(hash+"\n"), 0644)
}
