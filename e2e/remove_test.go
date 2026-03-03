package e2e_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("line remove", func() {
	var dir string

	BeforeEach(func() {
		dir = tempRepo()
	})

	// RMV-1: Removes assembly-line blocks from pre-commit and post-commit hooks,
	// preserving any other hook content.
	It("removes assembly-line blocks from git hooks [RMV-1]", func() {
		lineOK(dir, "init")

		// Verify hooks are installed
		preCommit := readFile(dir, ".git/hooks/pre-commit")
		Expect(preCommit).To(ContainSubstring("line gate"))

		lineOK(dir, "remove")

		// Hooks files should still exist but have no assembly-line content
		preCommit = readFile(dir, ".git/hooks/pre-commit")
		Expect(preCommit).NotTo(ContainSubstring("line gate"))
		Expect(preCommit).NotTo(ContainSubstring("# >>> assembly-line >>>"))
		Expect(preCommit).NotTo(ContainSubstring("# <<< assembly-line <<<"))

		postCommit := readFile(dir, ".git/hooks/post-commit")
		Expect(postCommit).NotTo(ContainSubstring("line run"))
		Expect(postCommit).NotTo(ContainSubstring("# >>> assembly-line >>>"))
		Expect(postCommit).NotTo(ContainSubstring("# <<< assembly-line <<<"))
	})

	It("preserves other hook content when removing [RMV-1]", func() {
		// Install an existing hook first
		hooksDir := filepath.Join(dir, ".git", "hooks")
		err := os.MkdirAll(hooksDir, 0o755)
		Expect(err).NotTo(HaveOccurred())

		existing := "#!/bin/sh\necho 'my custom hook'\n"
		err = os.WriteFile(filepath.Join(hooksDir, "pre-commit"), []byte(existing), 0o755)
		Expect(err).NotTo(HaveOccurred())

		lineOK(dir, "init")

		// Confirm both exist
		preCommit := readFile(dir, ".git/hooks/pre-commit")
		Expect(preCommit).To(ContainSubstring("my custom hook"))
		Expect(preCommit).To(ContainSubstring("line gate"))

		lineOK(dir, "remove")

		// Custom hook content preserved, assembly-line content removed
		preCommit = readFile(dir, ".git/hooks/pre-commit")
		Expect(preCommit).To(ContainSubstring("my custom hook"))
		Expect(preCommit).NotTo(ContainSubstring("line gate"))
		Expect(preCommit).NotTo(ContainSubstring("# >>> assembly-line >>>"))
	})

	// RMV-2: Removes the /line-rebase and /line-preview skill directories.
	It("removes skill directories [RMV-2]", func() {
		lineOK(dir, "init")

		// Verify skills installed
		Expect(fileExists(dir, ".claude/skills/line-rebase/SKILL.md")).To(BeTrue())
		Expect(fileExists(dir, ".claude/skills/line-preview/SKILL.md")).To(BeTrue())

		lineOK(dir, "remove")

		// Skill directories should be gone
		Expect(fileExists(dir, ".claude/skills/line-rebase")).To(BeFalse())
		Expect(fileExists(dir, ".claude/skills/line-preview")).To(BeFalse())
	})

	It("does not remove other skills [RMV-2]", func() {
		lineOK(dir, "init")

		// Add a custom skill alongside
		customSkill := filepath.Join(dir, ".claude", "skills", "my-custom-skill")
		err := os.MkdirAll(customSkill, 0o755)
		Expect(err).NotTo(HaveOccurred())
		err = os.WriteFile(filepath.Join(customSkill, "SKILL.md"), []byte("# My Skill\n"), 0o644)
		Expect(err).NotTo(HaveOccurred())

		lineOK(dir, "remove")

		// Custom skill should still be there
		Expect(fileExists(dir, ".claude/skills/my-custom-skill/SKILL.md")).To(BeTrue())
	})

	// RMV-3: Removes the statusLine key from .claude/settings.json,
	// preserving other settings.
	It("removes statusLine from Claude Code settings [RMV-3]", func() {
		lineOK(dir, "init")

		// Verify statusLine is set
		data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
		Expect(err).NotTo(HaveOccurred())
		var settings map[string]any
		err = json.Unmarshal(data, &settings)
		Expect(err).NotTo(HaveOccurred())
		Expect(settings).To(HaveKey("statusLine"))

		lineOK(dir, "remove")

		// statusLine key should be gone
		data, err = os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
		Expect(err).NotTo(HaveOccurred())
		var after map[string]any
		err = json.Unmarshal(data, &after)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).NotTo(HaveKey("statusLine"))
	})

	It("preserves other Claude Code settings when removing statusLine [RMV-3]", func() {
		// Write existing settings before init
		settingsDir := filepath.Join(dir, ".claude")
		err := os.MkdirAll(settingsDir, 0o755)
		Expect(err).NotTo(HaveOccurred())
		err = os.WriteFile(
			filepath.Join(settingsDir, "settings.json"),
			[]byte(`{"model":"sonnet","theme":"dark"}`),
			0o644,
		)
		Expect(err).NotTo(HaveOccurred())

		lineOK(dir, "init")
		lineOK(dir, "remove")

		data, err := os.ReadFile(filepath.Join(settingsDir, "settings.json"))
		Expect(err).NotTo(HaveOccurred())
		var settings map[string]any
		err = json.Unmarshal(data, &settings)
		Expect(err).NotTo(HaveOccurred())

		Expect(settings).NotTo(HaveKey("statusLine"))
		Expect(settings["model"]).To(Equal("sonnet"))
		Expect(settings["theme"]).To(Equal("dark"))
	})

	// RMV-6: Removes the PostToolUse hook entry from .claude/settings.json.
	It("removes PostToolUse auto-rebase hook from settings [RMV-6]", func() {
		lineOK(dir, "init")

		// Verify hook is installed
		data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
		Expect(err).NotTo(HaveOccurred())
		var before map[string]any
		err = json.Unmarshal(data, &before)
		Expect(err).NotTo(HaveOccurred())
		Expect(before).To(HaveKey("hooks"))

		lineOK(dir, "remove")

		// Hook should be gone
		data, err = os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
		Expect(err).NotTo(HaveOccurred())
		var after map[string]any
		err = json.Unmarshal(data, &after)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).NotTo(HaveKey("hooks"))
	})

	It("preserves other hooks when removing auto-rebase hook [RMV-6]", func() {
		lineOK(dir, "init")

		// Add a custom hook alongside
		data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
		Expect(err).NotTo(HaveOccurred())
		var settings map[string]any
		err = json.Unmarshal(data, &settings)
		Expect(err).NotTo(HaveOccurred())

		hooks := settings["hooks"].(map[string]any)
		hooks["PreToolUse"] = []any{
			map[string]any{
				"matcher": "",
				"hooks":   []any{map[string]any{"type": "command", "command": "echo custom"}},
			},
		}
		settings["hooks"] = hooks
		out, err := json.MarshalIndent(settings, "", "  ")
		Expect(err).NotTo(HaveOccurred())
		err = os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), append(out, '\n'), 0o644)
		Expect(err).NotTo(HaveOccurred())

		lineOK(dir, "remove")

		data, err = os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
		Expect(err).NotTo(HaveOccurred())
		var after map[string]any
		err = json.Unmarshal(data, &after)
		Expect(err).NotTo(HaveOccurred())

		// hooks key should still exist with PreToolUse, but PostToolUse should be gone
		hooksAfter, ok := after["hooks"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(hooksAfter).To(HaveKey("PreToolUse"))
		Expect(hooksAfter).NotTo(HaveKey("PostToolUse"))
	})

	// RMV-4: Removes the assembly-line block from .gitignore, preserving other entries.
	It("removes assembly-line block from .gitignore [RMV-4]", func() {
		lineOK(dir, "init")

		gi := readFile(dir, ".gitignore")
		Expect(gi).To(ContainSubstring("/.line/"))

		lineOK(dir, "remove")

		gi = readFile(dir, ".gitignore")
		Expect(gi).NotTo(ContainSubstring("/.line/"))
		Expect(gi).NotTo(ContainSubstring("# >>> assembly-line >>>"))
		Expect(gi).NotTo(ContainSubstring("# <<< assembly-line <<<"))
	})

	It("preserves other .gitignore entries when removing [RMV-4]", func() {
		writeFile(dir, ".gitignore", "node_modules/\n*.log\n")

		lineOK(dir, "init")
		lineOK(dir, "remove")

		gi := readFile(dir, ".gitignore")
		Expect(gi).To(ContainSubstring("node_modules/"))
		Expect(gi).To(ContainSubstring("*.log"))
		Expect(gi).NotTo(ContainSubstring("/.line/"))
	})

	// RMV-5: Safe to run when assembly-line was never initialized.
	It("is a no-op when assembly-line was never initialized [RMV-5]", func() {
		out := lineOK(dir, "remove")
		Expect(out).To(ContainSubstring("removed"))
	})

	It("is idempotent - running remove twice succeeds [RMV-5]", func() {
		lineOK(dir, "init")
		lineOK(dir, "remove")
		lineOK(dir, "remove")
	})

	It("round-trips: init → remove → init works cleanly", func() {
		lineOK(dir, "init")
		lineOK(dir, "remove")
		lineOK(dir, "init")

		// Everything should be back
		preCommit := readFile(dir, ".git/hooks/pre-commit")
		Expect(preCommit).To(ContainSubstring("line gate"))

		postCommit := readFile(dir, ".git/hooks/post-commit")
		Expect(postCommit).To(ContainSubstring("line run"))

		Expect(fileExists(dir, ".claude/skills/line-rebase/SKILL.md")).To(BeTrue())
		Expect(fileExists(dir, ".claude/skills/line-preview/SKILL.md")).To(BeTrue())

		gi := readFile(dir, ".gitignore")
		Expect(gi).To(ContainSubstring("/.line/"))

		data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
		Expect(err).NotTo(HaveOccurred())
		var settings map[string]any
		err = json.Unmarshal(data, &settings)
		Expect(err).NotTo(HaveOccurred())
		Expect(settings).To(HaveKey("statusLine"))

		// And only one block each
		count := strings.Count(preCommit, "# >>> assembly-line >>>")
		Expect(count).To(Equal(1))
		count = strings.Count(gi, "# >>> assembly-line >>>")
		Expect(count).To(Equal(1))
	})
})
