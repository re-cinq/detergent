package e2e_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("line init", func() {
	var dir string

	BeforeEach(func() {
		dir = tempRepo()
	})

	// INIT-1: Appends a Git pre-commit hook invoking `line gate`
	// INIT-3: Appends a Git post-commit hook invoking `line run`
	It("installs pre-commit and post-commit hooks [INIT-1, INIT-3]", func() {
		lineOK(dir, "init")

		preCommit := readFile(dir, ".git/hooks/pre-commit")
		Expect(preCommit).To(ContainSubstring("line gate"))
		Expect(preCommit).To(ContainSubstring("# >>> assembly-line >>>"))
		Expect(preCommit).To(ContainSubstring("# <<< assembly-line <<<"))

		postCommit := readFile(dir, ".git/hooks/post-commit")
		Expect(postCommit).To(ContainSubstring("line run"))
		Expect(postCommit).To(ContainSubstring("# >>> assembly-line >>>"))
		Expect(postCommit).To(ContainSubstring("# <<< assembly-line <<<"))
	})

	// INIT-2: Preserves any existing Git pre-commit hooks
	It("preserves existing hook content [INIT-2]", func() {
		hooksDir := filepath.Join(dir, ".git", "hooks")
		err := os.MkdirAll(hooksDir, 0o755)
		Expect(err).NotTo(HaveOccurred())

		existing := "#!/bin/sh\necho 'existing hook'\n"
		err = os.WriteFile(filepath.Join(hooksDir, "pre-commit"), []byte(existing), 0o755)
		Expect(err).NotTo(HaveOccurred())

		lineOK(dir, "init")

		preCommit := readFile(dir, ".git/hooks/pre-commit")
		Expect(preCommit).To(ContainSubstring("existing hook"))
		Expect(preCommit).To(ContainSubstring("line gate"))
	})

	// INIT-4: Converges on the desired state - idempotent
	It("is idempotent - running init twice does not duplicate hooks [INIT-4]", func() {
		lineOK(dir, "init")
		lineOK(dir, "init")

		preCommit := readFile(dir, ".git/hooks/pre-commit")
		count := strings.Count(preCommit, "# >>> assembly-line >>>")
		Expect(count).To(Equal(1), "expected exactly one assembly-line block, got %d", count)

		postCommit := readFile(dir, ".git/hooks/post-commit")
		count = strings.Count(postCommit, "# >>> assembly-line >>>")
		Expect(count).To(Equal(1), "expected exactly one assembly-line block, got %d", count)
	})

	// INIT-7: Adds .gitignore entries for state directory
	It("adds .line/ to .gitignore [INIT-7]", func() {
		lineOK(dir, "init")

		gi := readFile(dir, ".gitignore")
		Expect(gi).To(ContainSubstring("/.line/"))
		Expect(gi).To(ContainSubstring("# >>> assembly-line >>>"))
		Expect(gi).To(ContainSubstring("# <<< assembly-line <<<"))
	})

	It("preserves existing .gitignore content [INIT-7]", func() {
		writeFile(dir, ".gitignore", "node_modules/\n*.log\n")

		lineOK(dir, "init")

		gi := readFile(dir, ".gitignore")
		Expect(gi).To(ContainSubstring("node_modules/"))
		Expect(gi).To(ContainSubstring("*.log"))
		Expect(gi).To(ContainSubstring("/.line/"))
	})

	It("is idempotent for .gitignore [INIT-7]", func() {
		lineOK(dir, "init")
		lineOK(dir, "init")

		gi := readFile(dir, ".gitignore")
		count := strings.Count(gi, "# >>> assembly-line >>>")
		Expect(count).To(Equal(1), "expected exactly one assembly-line block in .gitignore, got %d", count)
	})

	// INIT-5: Installs the /line-rebase skill
	It("installs the line-rebase skill [INIT-5]", func() {
		lineOK(dir, "init")

		skillFile := readFile(dir, ".claude/skills/line-rebase/SKILL.md")
		Expect(skillFile).To(ContainSubstring("/line-rebase"))
		Expect(skillFile).To(ContainSubstring("Procedure"))
		Expect(skillFile).To(ContainSubstring("line rebase"))
	})
})
