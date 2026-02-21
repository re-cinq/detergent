package acceptance_test

import (
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("line gate", func() {
	var tmpDir, repoDir string

	BeforeEach(func() {
		tmpDir, repoDir = setupTestRepo("gate-")
	})

	AfterEach(func() {
		cleanupTestRepo(repoDir, tmpDir)
	})

	writeGateConfig := func(repoDir, content string) {
		writeFile(filepath.Join(repoDir, "line.yaml"), content)
	}

	Context("with a passing gate", func() {
		BeforeEach(func() {
			writeGateConfig(repoDir, `gates:
  - name: lint
    run: "echo lint passed"
`)
		})

		It("exits with code 0", func() {
			cmd := exec.Command(binaryPath, "gate", "--path", filepath.Join(repoDir, "line.yaml"))
			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred())
		})

		It("prints the gate header", func() {
			cmd := exec.Command(binaryPath, "gate", "--path", filepath.Join(repoDir, "line.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("--- lint ---"))
		})
	})

	Context("with a failing gate", func() {
		BeforeEach(func() {
			writeGateConfig(repoDir, `gates:
  - name: lint
    run: "exit 1"
`)
		})

		It("exits with a non-zero code", func() {
			cmd := exec.Command(binaryPath, "gate", "--path", filepath.Join(repoDir, "line.yaml"))
			err := cmd.Run()
			Expect(err).To(HaveOccurred())
		})

		It("reports which gate failed", func() {
			cmd := exec.Command(binaryPath, "gate", "--path", filepath.Join(repoDir, "line.yaml"))
			output, _ := cmd.CombinedOutput()
			Expect(string(output)).To(ContainSubstring("lint"))
		})
	})

	Context("fail-fast behavior", func() {
		BeforeEach(func() {
			writeGateConfig(repoDir, `gates:
  - name: first
    run: "exit 1"
  - name: second
    run: "echo second ran"
`)
		})

		It("does not run the second gate after the first fails", func() {
			cmd := exec.Command(binaryPath, "gate", "--path", filepath.Join(repoDir, "line.yaml"))
			output, _ := cmd.CombinedOutput()
			out := string(output)
			Expect(out).To(ContainSubstring("--- first ---"))
			Expect(out).NotTo(ContainSubstring("--- second ---"))
		})
	})

	Context("with multiple passing gates", func() {
		BeforeEach(func() {
			writeGateConfig(repoDir, `gates:
  - name: lint
    run: "echo lint ok"
  - name: fmt
    run: "echo fmt ok"
`)
		})

		It("runs all gates in order", func() {
			cmd := exec.Command(binaryPath, "gate", "--path", filepath.Join(repoDir, "line.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			out := string(output)
			Expect(out).To(ContainSubstring("--- lint ---"))
			Expect(out).To(ContainSubstring("--- fmt ---"))
		})
	})

	Context("{staged} substitution", func() {
		BeforeEach(func() {
			writeGateConfig(repoDir, `gates:
  - name: check
    run: "echo {staged}"
`)
			// Stage a file
			writeFile(filepath.Join(repoDir, "new.txt"), "new content\n")
			runGit(repoDir, "add", "new.txt")
		})

		It("substitutes staged file names into the run command", func() {
			cmd := exec.Command(binaryPath, "gate", "--path", filepath.Join(repoDir, "line.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("new.txt"))
		})
	})

	Context("with no gates configured", func() {
		BeforeEach(func() {
			writeGateConfig(repoDir, `agent:
  command: "echo"

stations:
  - name: security
    prompt: "check"
`)
		})

		It("exits with code 0", func() {
			cmd := exec.Command(binaryPath, "gate", "--path", filepath.Join(repoDir, "line.yaml"))
			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred())
		})

		It("prints a message about no gates", func() {
			cmd := exec.Command(binaryPath, "gate", "--path", filepath.Join(repoDir, "line.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("No gates configured"))
		})
	})

	Context("gates-only config (no agent/stations)", func() {
		BeforeEach(func() {
			writeGateConfig(repoDir, `gates:
  - name: lint
    run: "echo lint ok"
`)
		})

		It("works without agent or stations", func() {
			cmd := exec.Command(binaryPath, "gate", "--path", filepath.Join(repoDir, "line.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("--- lint ---"))
		})
	})
})

var _ = Describe("line gate validate", func() {
	Context("with valid gates in config", func() {
		It("passes validation", func() {
			cmd := exec.Command(binaryPath, "validate", "--path", testdataPath("gates_valid.yaml"))
			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("with missing gate fields", func() {
		It("reports missing name and run", func() {
			cmd := exec.Command(binaryPath, "validate", "--path", testdataPath("gates_missing_fields.yaml"))
			output, _ := cmd.CombinedOutput()
			out := string(output)
			Expect(out).To(ContainSubstring("name is required"))
			Expect(out).To(ContainSubstring("run is required"))
		})
	})

	Context("with duplicate gate names", func() {
		It("reports duplicate names", func() {
			cmd := exec.Command(binaryPath, "validate", "--path", testdataPath("gates_duplicate_names.yaml"))
			output, _ := cmd.CombinedOutput()
			Expect(string(output)).To(ContainSubstring("duplicate name"))
		})
	})
})
