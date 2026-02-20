package acceptance_test

import (
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("line viz", func() {
	Context("with a simple chain (main -> security -> docs)", func() {
		It("exits with code 0", func() {
			cmd := exec.Command(binaryPath, "viz", "--path", testdataPath("valid.yaml"))
			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred())
		})

		It("shows the source branch", func() {
			cmd := exec.Command(binaryPath, "viz", "--path", testdataPath("valid.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("[main]"))
		})

		It("shows concern names in order", func() {
			cmd := exec.Command(binaryPath, "viz", "--path", testdataPath("valid.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			out := string(output)
			Expect(out).To(ContainSubstring("security"))
			Expect(out).To(ContainSubstring("docs"))
		})
	})

	Context("with a longer chain", func() {
		It("shows all concerns", func() {
			cmd := exec.Command(binaryPath, "viz", "--path", testdataPath("complex_graph.yaml"))
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			out := string(output)
			Expect(out).To(ContainSubstring("security"))
			Expect(out).To(ContainSubstring("style"))
			Expect(out).To(ContainSubstring("docs"))
			Expect(out).To(ContainSubstring("final-review"))
		})
	})
})
