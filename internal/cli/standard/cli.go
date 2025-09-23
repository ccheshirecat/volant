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
        Use:   "viper",
        Short: "Viper command-line interface",
        Long:  "Viper CLI provides access to the orchestrator, browser agents, and tooling.",
        RunE: func(cmd *cobra.Command, args []string) error {
            return cmd.Help()
        },
    }

    cmd.PersistentFlags().StringP("api", "a", envOrDefault("VIPER_API_BASE", "http://127.0.0.1:7777"), "viper-server base URL")

    cmd.AddCommand(newVersionCmd())
    cmd.AddCommand(newVMsCmd())
    cmd.AddCommand(newSetupCmd())
    return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Viper client version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "Viper CLI (prototype)\n")
		},
	}
}
