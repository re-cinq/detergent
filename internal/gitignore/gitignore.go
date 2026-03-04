package gitignore

import (
	"fmt"
	"path/filepath"

	"github.com/re-cinq/assembly-line/internal/markers"
)

func block() string {
	return fmt.Sprintf(`%s
/.line/
%s`, markers.Start, markers.End)
}

// Remove removes the assembly-line block from .gitignore.
func Remove(repoDir string) error {
	path := filepath.Join(repoDir, ".gitignore")
	if err := markers.RemoveFromFile(path, "", 0o644); err != nil {
		return fmt.Errorf(".gitignore: %w", err)
	}
	return nil
}

// Install adds the assembly-line gitignore entries to .gitignore in the given repo,
// creating the file if needed. The block is idempotent: if markers already
// exist, the content between them is replaced.
func Install(repoDir string) error {
	path := filepath.Join(repoDir, ".gitignore")
	if err := markers.InsertInFile(path, block(), "", 0o644); err != nil {
		return fmt.Errorf(".gitignore: %w", err)
	}
	return nil
}
