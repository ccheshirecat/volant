package standard

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Execute runs the Cobra-based CLI entry point.
func Execute() error {
	return newRootCmd().Execute()
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "volar",
		Short: "volar command-line interface",
		Long: `volar provides access to the VOLANT control plane.

Core commands:
  vms       Manage microVMs
  plugins   Install/remove plugin manifests
  setup     Helper for host networking/service configuration
  console   Inspect or attach to VM consoles
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.PersistentFlags().StringP("api", "a", envOrDefault("VOLANT_API_BASE", "http://127.0.0.1:7777"), "volantd base URL")

	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newVMsCmd())
	cmd.AddCommand(newPluginsCmd())
	cmd.AddCommand(newSetupCmd())
	cmd.AddCommand(newDeploymentsCmd())
	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the volar client version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "volar CLI (prototype)\n")
		},
	}
}
