package main

import (
	"fmt"
	"os"

	"github.com/volantvm/volant/internal/cli/standard"
)

func main() {
	// TUI temporarily disabled - using standard CLI for all invocations
	// When invoked without arguments, will show help instead of TUI

	if err := standard.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "command error: %v\n", err)
		os.Exit(1)
	}
}
