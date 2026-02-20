package engine

import (
	"testing"

	ignore "github.com/sabhiram/go-gitignore"
)

func compilePatterns(patterns []string) *ignore.GitIgnore {
	return ignore.CompileIgnoreLines(patterns...)
}

func TestFilesMatchIgnorePatterns(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		patterns []string
		useNilGI bool
		want     bool
	}{
		{
			name:     "nil matcher returns false",
			files:    []string{"foo.go"},
			useNilGI: true,
			want:     false,
		},
		{
			name:     "empty file list returns false",
			files:    []string{},
			patterns: []string{"*.md"},
			want:     false,
		},
		{
			name:     "all files match patterns",
			files:    []string{"docs/README.md", "docs/guide.md"},
			patterns: []string{"docs/"},
			want:     true,
		},
		{
			name:     "mixed files returns false",
			files:    []string{"docs/README.md", "main.go"},
			patterns: []string{"docs/"},
			want:     false,
		},
		{
			name:     ".lineignore in file list always returns false",
			files:    []string{".lineignore"},
			patterns: []string{".lineignore"},
			want:     false,
		},
		{
			name:     ".lineignore mixed with other ignored files returns false",
			files:    []string{".beads/issues.jsonl", ".lineignore"},
			patterns: []string{".beads/", ".lineignore"},
			want:     false,
		},
		{
			name:     "glob patterns work",
			files:    []string{"README.md", "CHANGELOG.md"},
			patterns: []string{"*.md"},
			want:     true,
		},
		{
			name:     "nested paths with doublestar",
			files:    []string{".beads/issues.jsonl", ".beads/config.json"},
			patterns: []string{".beads/"},
			want:     true,
		},
		{
			name:     "multiple patterns",
			files:    []string{".beads/issues.jsonl", "docs/guide.md", ".github/workflows/ci.yml"},
			patterns: []string{".beads/", "docs/", ".github/"},
			want:     true,
		},
		{
			name:     "unmatched file among matched",
			files:    []string{".beads/issues.jsonl", "src/main.go"},
			patterns: []string{".beads/"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gi *ignore.GitIgnore
			if !tt.useNilGI {
				gi = compilePatterns(tt.patterns)
			}
			got := filesMatchIgnorePatterns(tt.files, gi)
			if got != tt.want {
				t.Errorf("filesMatchIgnorePatterns(%v) = %v, want %v", tt.files, got, tt.want)
			}
		})
	}
}
