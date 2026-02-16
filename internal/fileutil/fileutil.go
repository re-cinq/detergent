package fileutil

import (
	"encoding/json"
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

// WriteJSON marshals data to indented JSON and writes it to path with a trailing newline.
func WriteJSON(path string, data interface{}) error {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(bytes, '\n'), 0644)
}
