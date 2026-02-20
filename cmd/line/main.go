package main

import (
	"os"

	"github.com/re-cinq/assembly-line/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
