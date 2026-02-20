package acceptance_test

import (
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CLI", func() {
	Describe("line --help", func() {
		It("exits with code 0", func() {
			cmd := exec.Command(binaryPath, "--help")
			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred())
		})

		It("shows the tool description", func() {
			cmd := exec.Command(binaryPath, "--help")
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("orchestrates coding agents"))
		})

		It("lists available commands", func() {
			cmd := exec.Command(binaryPath, "--help")
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(ContainSubstring("Available Commands"))
			Expect(string(output)).To(ContainSubstring("version"))
		})
	})

	Describe("line version", func() {
		It("exits with code 0", func() {
			cmd := exec.Command(binaryPath, "version")
			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred())
		})

		It("prints a version string", func() {
			cmd := exec.Command(binaryPath, "version")
			output, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(output)).To(MatchRegexp(`line \S+`))
		})
	})
})
