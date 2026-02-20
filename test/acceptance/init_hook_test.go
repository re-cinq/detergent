package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("line init (pre-commit hook)", func() {
	var tmpDir, repoDir string

	BeforeEach(func() {
		tmpDir, repoDir = setupTestRepo("init-hook-")
	})

	AfterEach(func() {
		cleanupTestRepo(repoDir, tmpDir)
	})

	Context("when gates are configured", func() {
		BeforeEach(func() {
			writeFile(filepath.Join(repoDir, "line.yaml"), `gates:
  - name: lint
    run: "echo ok"
`)
		})

		It("installs the pre-commit hook", func() {
			cmd := exec.Command(binaryPath, "init", repoDir, "--path", filepath.Join(repoDir, "line.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "init failed: %s", string(output))

			hookPath := filepath.Join(repoDir, ".git", "hooks", "pre-commit")
			info, err := os.Stat(hookPath)
			Expect(err).NotTo(HaveOccurred(), "hook should exist")
			Expect(info.Mode().Perm() & 0o111).NotTo(BeZero(), "hook should be executable")
		})

		It("prints a message about the hook", func() {
			cmd := exec.Command(binaryPath, "init", repoDir, "--path", filepath.Join(repoDir, "line.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "init failed: %s", string(output))
			Expect(string(output)).To(ContainSubstring("hook"))
		})
	})

	Context("when a pre-commit hook already exists", func() {
		BeforeEach(func() {
			writeFile(filepath.Join(repoDir, "line.yaml"), `gates:
  - name: lint
    run: "echo ok"
`)
			hookDir := filepath.Join(repoDir, ".git", "hooks")
			Expect(os.MkdirAll(hookDir, 0o755)).To(Succeed())
			writeFile(filepath.Join(hookDir, "pre-commit"), "#!/bin/sh\necho existing\n")
			Expect(os.Chmod(filepath.Join(hookDir, "pre-commit"), 0o755)).To(Succeed())
		})

		It("does not overwrite the existing hook", func() {
			cmd := exec.Command(binaryPath, "init", repoDir, "--path", filepath.Join(repoDir, "line.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "init failed: %s", string(output))

			hookContent, err := os.ReadFile(filepath.Join(repoDir, ".git", "hooks", "pre-commit"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(hookContent)).To(ContainSubstring("existing"))
		})

		It("prints a skip message", func() {
			cmd := exec.Command(binaryPath, "init", repoDir, "--path", filepath.Join(repoDir, "line.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "init failed: %s", string(output))
			Expect(string(output)).To(ContainSubstring("skip"))
		})
	})

	Context("when no gates are configured", func() {
		BeforeEach(func() {
			writeFile(filepath.Join(repoDir, "line.yaml"), `agent:
  command: "echo"

concerns:
  - name: security
    prompt: "check"
`)
		})

		It("does not install a pre-commit hook", func() {
			cmd := exec.Command(binaryPath, "init", repoDir, "--path", filepath.Join(repoDir, "line.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "init failed: %s", string(output))

			hookPath := filepath.Join(repoDir, ".git", "hooks", "pre-commit")
			_, err = os.Stat(hookPath)
			Expect(os.IsNotExist(err)).To(BeTrue(), "hook should not exist")
		})
	})
})
