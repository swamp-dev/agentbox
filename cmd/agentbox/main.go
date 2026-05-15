// Agentbox is a CLI tool for running Docker-sandboxed AI coding agents.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/swamp-dev/agentbox/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		switch {
		case errors.Is(err, cli.ErrWaitTimeout):
			os.Exit(2)
		default:
			os.Exit(1)
		}
	}
}
