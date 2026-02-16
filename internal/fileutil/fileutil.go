package fileutil

import (
	"fmt"
	"os"
)

// EnsureDir creates a directory and all parent directories with 0755 permissions.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// LogError writes an error message to stderr.
func LogError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
