// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

package standard

import "github.com/spf13/cobra"

// newBrowsersCmd is no longer available in the engine CLI.
func newBrowsersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "browsers",
		Short: "Browser commands moved to plugin distribution",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("browser automation commands now live in the browser plugin CLI")
			cmd.Println("Install the browser plugin distribution and use its CLI wrapper.")
			return nil
		},
	}
}
