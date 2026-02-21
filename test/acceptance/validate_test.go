package acceptance_test

import (
	"os/exec"
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func testdataPath(name string) string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "testdata", name)
}

var _ = Describe("line validate", func() {
	Context("with a valid config", func() {
		It("exits with code 0", func() {
			cmd := exec.Command(binaryPath, "validate", "--path", testdataPath("valid.yaml"))
			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred())
		})

		It("prints a success message", func() {
			cmd := exec.Command(binaryPath, "validate", "--path", testdataPath("valid.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("valid"))
		})
	})

	Context("with invalid YAML syntax", func() {
		It("exits with a non-zero code", func() {
			cmd := exec.Command(binaryPath, "validate", "--path", testdataPath("invalid_yaml.yaml"))
			err := cmd.Run()
			Expect(err).To(HaveOccurred())
		})

		It("reports a YAML parse error", func() {
			cmd := exec.Command(binaryPath, "validate", "--path", testdataPath("invalid_yaml.yaml"))
			output, _ := cmd.CombinedOutput()
			Expect(string(output)).To(ContainSubstring("YAML"))
		})
	})

	Context("with missing required fields", func() {
		It("exits with a non-zero code", func() {
			cmd := exec.Command(binaryPath, "validate", "--path", testdataPath("missing_fields.yaml"))
			err := cmd.Run()
			Expect(err).To(HaveOccurred())
		})

		It("reports each missing field", func() {
			cmd := exec.Command(binaryPath, "validate", "--path", testdataPath("missing_fields.yaml"))
			output, _ := cmd.CombinedOutput()
			out := string(output)
			Expect(out).To(ContainSubstring("agent.command"))
			Expect(out).To(ContainSubstring("name is required"))
			Expect(out).To(ContainSubstring("prompt is required"))
		})
	})

	Context("with a cycle in the station graph", func() {
		It("exits with a non-zero code", func() {
			cmd := exec.Command(binaryPath, "validate", "--path", testdataPath("cycle.yaml"))
			err := cmd.Run()
			Expect(err).To(HaveOccurred())
		})

		It("reports the cycle", func() {
			cmd := exec.Command(binaryPath, "validate", "--path", testdataPath("cycle.yaml"))
			output, _ := cmd.CombinedOutput()
			Expect(string(output)).To(ContainSubstring("cycle detected"))
		})
	})

	Context("with a nonexistent file", func() {
		It("exits with a non-zero code", func() {
			cmd := exec.Command(binaryPath, "validate", "--path", "/tmp/does-not-exist.yaml")
			err := cmd.Run()
			Expect(err).To(HaveOccurred())
		})
	})
})
