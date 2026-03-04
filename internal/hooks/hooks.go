package hooks

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/re-cinq/assembly-line/internal/markers"
)

const shebang = "#!/bin/sh"

func preCommitBlock() string {
	return fmt.Sprintf(`%s
line gate
%s`, markers.Start, markers.End)
}

func postCommitBlock() string {
	return fmt.Sprintf(`%s
line run &
%s`, markers.Start, markers.End)
}

// Install installs or updates the assembly-line hooks in the given git repo.
func Install(repoDir string) error {
	hooksDir := filepath.Join(repoDir, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("creating hooks dir: %w", err)
	}

	if err := installHook(hooksDir, "pre-commit", preCommitBlock()); err != nil {
		return err
	}
	if err := installHook(hooksDir, "post-commit", postCommitBlock()); err != nil {
		return err
	}
	return nil
}

// Remove removes assembly-line blocks from pre-commit and post-commit hooks.
func Remove(repoDir string) error {
	hooksDir := filepath.Join(repoDir, ".git", "hooks")
	if err := removeHook(hooksDir, "pre-commit"); err != nil {
		return err
	}
	if err := removeHook(hooksDir, "post-commit"); err != nil {
		return err
	}
	return nil
}

func removeHook(hooksDir, name string) error {
	path := filepath.Join(hooksDir, name)
	if err := markers.RemoveFromFile(path, shebang+"\n", 0o755); err != nil {
		return fmt.Errorf("%s hook: %w", name, err)
	}
	return nil
}

func installHook(hooksDir, name, block string) error {
	path := filepath.Join(hooksDir, name)
	if err := markers.InsertInFile(path, block, shebang, 0o755); err != nil {
		return fmt.Errorf("%s hook: %w", name, err)
	}
	return nil
}
