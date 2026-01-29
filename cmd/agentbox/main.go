// Agentbox is a CLI tool for running Docker-sandboxed AI coding agents.
package main

import (
	"fmt"
	"os"

	"github.com/swamp-dev/agentbox/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
