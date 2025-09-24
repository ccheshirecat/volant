package standard

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/ccheshirecat/viper/internal/cli/client"
)

func newVMsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vms",
		Short: "Manage microVMs",
	}

	cmd.AddCommand(newVMsListCmd())
	cmd.AddCommand(newVMsCreateCmd())
	cmd.AddCommand(newVMsDeleteCmd())
	cmd.AddCommand(newVMsGetCmd())
	cmd.AddCommand(newVMsWatchCmd())
	return cmd
}

func newVMsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List microVMs",
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()

			vms, err := api.ListVMs(ctx)
			if err != nil {
				return err
			}
			if len(vms) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No VMs found")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-10s %-15s %-20s %-6s %-6s\n", "NAME", "STATUS", "IP", "MAC", "CPU", "MEM")
			for _, vm := range vms {
				fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-10s %-15s %-20s %-6d %-6d\n", vm.Name, vm.Status, vm.IPAddress, vm.MACAddress, vm.CPUCores, vm.MemoryMB)
			}
			return nil
		},
	}
	return cmd
}

func newVMsGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Show microVM details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()

			vm, err := api.GetVM(ctx, args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Name: %s\nStatus: %s\nIP: %s\nMAC: %s\nCPU: %d\nMemory: %d MB\n", vm.Name, vm.Status, vm.IPAddress, vm.MACAddress, vm.CPUCores, vm.MemoryMB)
			if vm.PID != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "PID: %d\n", *vm.PID)
			}
			if vm.KernelCmdline != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Kernel Cmdline: %s\n", vm.KernelCmdline)
			}
			return nil
		},
	}
	return cmd
}

func newVMsCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a microVM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cpu, err := cmd.Flags().GetInt("cpu")
			if err != nil {
				return err
			}
			mem, err := cmd.Flags().GetInt("memory")
			if err != nil {
				return err
			}
			kernelCmdline, err := cmd.Flags().GetString("kernel-cmdline")
			if err != nil {
				return err
			}

			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			vm, err := api.CreateVM(ctx, client.CreateVMRequest{
				Name:          args[0],
				CPUCores:      cpu,
				MemoryMB:      mem,
				KernelCmdline: kernelCmdline,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "VM %s created with IP %s\n", vm.Name, vm.IPAddress)
			return nil
		},
	}
	cmd.Flags().Int("cpu", 2, "Number of virtual CPU cores")
	cmd.Flags().Int("memory", 2048, "Memory (MB)")
	cmd.Flags().String("kernel-cmdline", "", "Additional kernel cmdline parameters")
	return cmd
}

func newVMsDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a microVM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()

			if err := api.DeleteVM(ctx, args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "VM %s deleted\n", args[0])
			return nil
		},
	}
	return cmd
}

func newVMsWatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Stream microVM lifecycle events",
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			return api.WatchVMEvents(ctx, func(ev client.VMEvent) {
				target := cmd.OutOrStdout()
				fmt.Fprintf(target, "%s	%s	%s	%s\n", ev.Timestamp.Format(time.RFC3339), ev.Type, ev.Name, ev.Message)
			})
		},
	}
	return cmd
}

func clientFromCmd(cmd *cobra.Command) (*client.Client, error) {
	base, err := cmd.Root().PersistentFlags().GetString("api")
	if err != nil {
		base = envOrDefault("VIPER_API_BASE", "http://127.0.0.1:7777")
	}
	return client.New(base)
}
