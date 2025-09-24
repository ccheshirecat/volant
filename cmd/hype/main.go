package main

import (
	"fmt"
	"os"

	"github.com/ccheshirecat/overhyped/internal/cli/standard"
	"github.com/ccheshirecat/overhyped/internal/cli/tui"
)

func main() {
	if len(os.Args) == 1 {
		if err := tui.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "tui error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := standard.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "command error: %v\n", err)
		os.Exit(1)
	}
}
