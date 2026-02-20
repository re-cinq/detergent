package fileutil

import "path/filepath"

// LineSubdir builds a path to a subdirectory within .line.
func LineSubdir(repoDir, subdir string) string {
	return filepath.Join(repoDir, ".line", subdir)
}

// ClaudeDir returns the .claude directory path for a repository.
func ClaudeDir(repoDir string) string {
	return filepath.Join(repoDir, ".claude")
}

// ClaudeSubpath returns a path within the .claude directory.
func ClaudeSubpath(repoDir, subpath string) string {
	return filepath.Join(repoDir, ".claude", subpath)
}
