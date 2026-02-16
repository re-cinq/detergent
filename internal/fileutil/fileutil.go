package fileutil

import "os"

// EnsureDir creates a directory and all parent directories with 0755 permissions.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}
