package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var binaryPath string

func TestAcceptance(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Acceptance Suite")
}

var _ = BeforeSuite(func() {
	// Build the binary once for all acceptance tests
	_, thisFile, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	binaryPath = filepath.Join(projectRoot, "bin", "line-test")

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/line")
	cmd.Dir = projectRoot
	cmd.Env = append(cmd.Environ(), "CGO_ENABLED=0")
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "Failed to build binary: %s", string(output))
})

// setupTestRepo initializes a git repository with an initial commit for testing.
// Returns the tmpDir and repoDir paths.
func setupTestRepo(namePrefix string) (tmpDir, repoDir string) {
	var err error
	tmpDir, err = os.MkdirTemp("", namePrefix)
	Expect(err).NotTo(HaveOccurred())

	repoDir = filepath.Join(tmpDir, "repo")
	runGit(tmpDir, "init", repoDir)
	runGit(repoDir, "checkout", "-b", "main")
	writeFile(filepath.Join(repoDir, "hello.txt"), "hello\n")
	runGit(repoDir, "add", "hello.txt")
	runGit(repoDir, "commit", "-m", "initial commit")

	return tmpDir, repoDir
}

// cleanupTestRepo cleans up git worktrees and removes the temporary directory.
func cleanupTestRepo(repoDir, tmpDir string) {
	exec.Command("git", "-C", repoDir, "worktree", "prune").Run()
	os.RemoveAll(tmpDir)
}

// runGit executes a git command and fails the test if it errors.
func runGit(dir string, args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "git %v: %s", args, string(out))
}

// runGitOutput executes a git command and returns its output, failing if it errors.
func runGitOutput(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "git %v: %s", args, string(out))
	return string(out)
}

// writeFile creates directories as needed and writes content to a file.
func writeFile(path, content string) {
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	err = os.WriteFile(path, []byte(content), 0644)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}
