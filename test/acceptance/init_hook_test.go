package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

		It("injects the gate block while preserving original content", func() {
			cmd := exec.Command(binaryPath, "init", repoDir, "--path", filepath.Join(repoDir, "line.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "init failed: %s", string(output))

			hookContent, err := os.ReadFile(filepath.Join(repoDir, ".git", "hooks", "pre-commit"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(hookContent)).To(ContainSubstring("echo existing"))
			Expect(string(hookContent)).To(ContainSubstring("# BEGIN line gate"))
			Expect(string(hookContent)).To(ContainSubstring("line gate || exit 1"))
		})

		It("prints an injection message", func() {
			cmd := exec.Command(binaryPath, "init", repoDir, "--path", filepath.Join(repoDir, "line.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "init failed: %s", string(output))
			Expect(string(output)).To(ContainSubstring("injected line gate"))
		})
	})

	Context("when the gate block is already injected", func() {
		BeforeEach(func() {
			writeFile(filepath.Join(repoDir, "line.yaml"), `gates:
  - name: lint
    run: "echo ok"
`)
			hookDir := filepath.Join(repoDir, ".git", "hooks")
			Expect(os.MkdirAll(hookDir, 0o755)).To(Succeed())
			writeFile(filepath.Join(hookDir, "pre-commit"), "#!/bin/sh\necho existing\n\n# BEGIN line gate\nif command -v line >/dev/null 2>&1; then\n    line gate || exit 1\nfi\n# END line gate\n")
			Expect(os.Chmod(filepath.Join(hookDir, "pre-commit"), 0o755)).To(Succeed())
		})

		It("is idempotent — does not duplicate the block", func() {
			cmd := exec.Command(binaryPath, "init", repoDir, "--path", filepath.Join(repoDir, "line.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "init failed: %s", string(output))

			hookContent, err := os.ReadFile(filepath.Join(repoDir, ".git", "hooks", "pre-commit"))
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.Count(string(hookContent), "# BEGIN line gate")).To(Equal(1))
		})

		It("prints a skip message", func() {
			cmd := exec.Command(binaryPath, "init", repoDir, "--path", filepath.Join(repoDir, "line.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "init failed: %s", string(output))
			Expect(string(output)).To(ContainSubstring("already present"))
		})
	})

	Context("when the existing hook ends with exit 0", func() {
		BeforeEach(func() {
			writeFile(filepath.Join(repoDir, "line.yaml"), `gates:
  - name: lint
    run: "echo ok"
`)
			hookDir := filepath.Join(repoDir, ".git", "hooks")
			Expect(os.MkdirAll(hookDir, 0o755)).To(Succeed())
			writeFile(filepath.Join(hookDir, "pre-commit"), "#!/bin/sh\necho existing\nexit 0\n")
			Expect(os.Chmod(filepath.Join(hookDir, "pre-commit"), 0o755)).To(Succeed())
		})

		It("injects the gate block before the final exit 0", func() {
			cmd := exec.Command(binaryPath, "init", repoDir, "--path", filepath.Join(repoDir, "line.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "init failed: %s", string(output))

			hookContent, err := os.ReadFile(filepath.Join(repoDir, ".git", "hooks", "pre-commit"))
			Expect(err).NotTo(HaveOccurred())
			content := string(hookContent)
			gateIdx := strings.Index(content, "# BEGIN line gate")
			exitIdx := strings.LastIndex(content, "exit 0\n")
			Expect(gateIdx).To(BeNumerically("<", exitIdx), "gate block should appear before final exit 0")
		})
	})

	Context("when no gates are configured", func() {
		BeforeEach(func() {
			writeFile(filepath.Join(repoDir, "line.yaml"), `agent:
  command: "echo"

stations:
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

var _ = Describe("line init (post-commit hook)", func() {
	var tmpDir, repoDir string

	BeforeEach(func() {
		tmpDir, repoDir = setupTestRepo("init-postcommit-")
	})

	AfterEach(func() {
		cleanupTestRepo(repoDir, tmpDir)
	})

	Context("when stations are configured", func() {
		BeforeEach(func() {
			writeFile(filepath.Join(repoDir, "line.yaml"), `agent:
  command: "echo"

stations:
  - name: security
    prompt: "check"
`)
		})

		It("installs the post-commit hook", func() {
			cmd := exec.Command(binaryPath, "init", repoDir, "--path", filepath.Join(repoDir, "line.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "init failed: %s", string(output))

			hookPath := filepath.Join(repoDir, ".git", "hooks", "post-commit")
			info, err := os.Stat(hookPath)
			Expect(err).NotTo(HaveOccurred(), "hook should exist")
			Expect(info.Mode().Perm() & 0o111).NotTo(BeZero(), "hook should be executable")

			content, err := os.ReadFile(hookPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("# BEGIN line runner"))
			Expect(string(content)).To(ContainSubstring("line trigger"))
		})
	})

	Context("when post-commit hook is already present", func() {
		BeforeEach(func() {
			writeFile(filepath.Join(repoDir, "line.yaml"), `agent:
  command: "echo"

stations:
  - name: security
    prompt: "check"
`)
		})

		It("is idempotent — does not duplicate the block", func() {
			initCmd := func() {
				cmd := exec.Command(binaryPath, "init", repoDir, "--path", filepath.Join(repoDir, "line.yaml"))
				output, err := cmd.CombinedOutput()
				Expect(err).NotTo(HaveOccurred(), "init failed: %s", string(output))
			}

			// Run init twice
			initCmd()
			initCmd()

			hookContent, err := os.ReadFile(filepath.Join(repoDir, ".git", "hooks", "post-commit"))
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.Count(string(hookContent), "# BEGIN line runner")).To(Equal(1))
		})
	})

	Context("when a post-commit hook already exists", func() {
		BeforeEach(func() {
			writeFile(filepath.Join(repoDir, "line.yaml"), `agent:
  command: "echo"

stations:
  - name: security
    prompt: "check"
`)
			hookDir := filepath.Join(repoDir, ".git", "hooks")
			Expect(os.MkdirAll(hookDir, 0o755)).To(Succeed())
			writeFile(filepath.Join(hookDir, "post-commit"), "#!/bin/sh\necho existing-hook\n")
			Expect(os.Chmod(filepath.Join(hookDir, "post-commit"), 0o755)).To(Succeed())
		})

		It("injects the runner block while preserving original content", func() {
			cmd := exec.Command(binaryPath, "init", repoDir, "--path", filepath.Join(repoDir, "line.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "init failed: %s", string(output))

			hookContent, err := os.ReadFile(filepath.Join(repoDir, ".git", "hooks", "post-commit"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(hookContent)).To(ContainSubstring("echo existing-hook"))
			Expect(string(hookContent)).To(ContainSubstring("# BEGIN line runner"))
			Expect(string(hookContent)).To(ContainSubstring("line trigger"))
		})
	})

	Context("when no stations are configured", func() {
		BeforeEach(func() {
			writeFile(filepath.Join(repoDir, "line.yaml"), `gates:
  - name: lint
    run: "echo ok"
`)
		})

		It("does not install a post-commit hook", func() {
			cmd := exec.Command(binaryPath, "init", repoDir, "--path", filepath.Join(repoDir, "line.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "init failed: %s", string(output))

			hookPath := filepath.Join(repoDir, ".git", "hooks", "post-commit")
			_, err = os.Stat(hookPath)
			Expect(os.IsNotExist(err)).To(BeTrue(), "hook should not exist when no stations")
		})
	})
})
