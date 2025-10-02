package main

import (
	"fmt"
	"os"

	"github.com/ccheshirecat/volant/internal/cli/standard"
	// TODO: TUI removed temporarily for task #6 - needs update for dynamic OpenAPI-based operations (task #8)
	// "github.com/ccheshirecat/volant/internal/cli/tui"
)

func main() {
	// TODO: TUI disabled temporarily for task #6 - will be updated in task #8 for feature parity
	// if len(os.Args) == 1 {
	// 	if err := tui.Run(); err != nil {
	// 		fmt.Fprintf(os.Stderr, "tui error: %v\n", err)
	// 		os.Exit(1)
	// 	}
	// 	return
	// }

	if err := standard.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "command error: %v\n", err)
		os.Exit(1)
	}
}
