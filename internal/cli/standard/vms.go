package standard

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ccheshirecat/volant/internal/cli/client"
)

func encodeAsJSON(out io.Writer, payload interface{}) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func decodeBase64(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(data)
}

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
	cmd.AddCommand(newVMsNavigateCmd())
	cmd.AddCommand(newVMsScreenshotCmd())
	cmd.AddCommand(newVMsScrapeCmd())
	cmd.AddCommand(newVMsExecCmd())
	cmd.AddCommand(newVMsGraphQLCmd())
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

func newVMsNavigateCmd() *cobra.Command {
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "navigate <vm> <url>",
		Short: "Navigate a VM's browser to a URL",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			if timeout <= 0 {
				timeout = 60 * time.Second
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			payload := client.NavigateActionRequest{
				URL:       args[1],
				TimeoutMs: int64(timeout / time.Millisecond),
			}

			if err := api.NavigateVM(ctx, args[0], payload); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Navigation requested for %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "Action timeout")
	return cmd
}

func newVMsScreenshotCmd() *cobra.Command {
	var outputPath string
	var fullPage bool
	var format string
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "screenshot <vm>",
		Short: "Capture a screenshot from a VM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			if timeout <= 0 {
				timeout = 60 * time.Second
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			payload := client.ScreenshotActionRequest{
				FullPage:  fullPage,
				Format:    format,
				TimeoutMs: int64(timeout / time.Millisecond),
			}

			resp, err := api.ScreenshotVM(ctx, args[0], payload)
			if err != nil {
				return err
			}

			data, decodeErr := decodeBase64(resp.Data)
			if decodeErr != nil {
				return decodeErr
			}

			path := outputPath
			if strings.TrimSpace(path) == "" {
				suffix := resp.Format
				if suffix == "" {
					suffix = payload.Format
				}
				if suffix == "" {
					suffix = "png"
				}
				path = fmt.Sprintf("%s_%d.%s", args[0], time.Now().Unix(), suffix)
			}

			if writeErr := os.WriteFile(path, data, 0o644); writeErr != nil {
				return writeErr
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Screenshot saved to %s (%d bytes)\n", path, len(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&outputPath, "output", "", "Destination file path")
	cmd.Flags().BoolVar(&fullPage, "full-page", false, "Capture full page")
	cmd.Flags().StringVar(&format, "format", "png", "Output format (png|jpeg)")
	cmd.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "Action timeout")

	return cmd
}

func newVMsScrapeCmd() *cobra.Command {
	var selector string
	var attribute string
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "scrape <vm>",
		Short: "Extract text or attribute from a VM page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(selector) == "" {
				return fmt.Errorf("selector is required")
			}
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			if timeout <= 0 {
				timeout = 60 * time.Second
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			payload := client.ScrapeActionRequest{
				Selector:  selector,
				Attribute: attribute,
				TimeoutMs: int64(timeout / time.Millisecond),
			}

			resp, err := api.ScrapeVM(ctx, args[0], payload)
			if err != nil {
				return err
			}

			output := struct {
				Value  interface{} `json:"value"`
				Exists bool        `json:"exists"`
			}{Value: resp.Value, Exists: resp.Exists}
			return encodeAsJSON(cmd.OutOrStdout(), output)
		},
	}

	cmd.Flags().StringVar(&selector, "selector", "", "CSS selector")
	cmd.Flags().StringVar(&attribute, "attr", "", "Attribute to read instead of text")
	cmd.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "Action timeout")

	return cmd
}

func newVMsExecCmd() *cobra.Command {
	var expression string
	var await bool
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "exec <vm>",
		Short: "Execute JavaScript in the VM context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(expression) == "" {
				return fmt.Errorf("expression is required")
			}
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			if timeout <= 0 {
				timeout = 60 * time.Second
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			payload := client.EvaluateActionRequest{
				Expression:   expression,
				AwaitPromise: await,
				TimeoutMs:    int64(timeout / time.Millisecond),
			}

			resp, err := api.EvaluateVM(ctx, args[0], payload)
			if err != nil {
				return err
			}
			return encodeAsJSON(cmd.OutOrStdout(), resp)
		},
	}

	cmd.Flags().StringVarP(&expression, "expression", "e", "", "JavaScript expression to evaluate")
	cmd.Flags().BoolVar(&await, "await", false, "Await returned promises")
	cmd.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "Action timeout")

	return cmd
}

func newVMsGraphQLCmd() *cobra.Command {
	var endpoint string
	var query string
	var variables string
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "graphql <vm>",
		Short: "Execute GraphQL request from VM context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(endpoint) == "" {
				return fmt.Errorf("endpoint is required")
			}
			if strings.TrimSpace(query) == "" {
				return fmt.Errorf("query is required")
			}

			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			if timeout <= 0 {
				timeout = 60 * time.Second
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			var vars map[string]interface{}
			if strings.TrimSpace(variables) != "" {
				if decodeErr := json.Unmarshal([]byte(variables), &vars); decodeErr != nil {
					return fmt.Errorf("invalid variables JSON: %w", decodeErr)
				}
			}

			payload := client.GraphQLActionRequest{
				Endpoint:  endpoint,
				Query:     query,
				Variables: vars,
				TimeoutMs: int64(timeout / time.Millisecond),
			}

			resp, err := api.GraphQLVM(ctx, args[0], payload)
			if err != nil {
				return err
			}
			return encodeAsJSON(cmd.OutOrStdout(), resp)
		},
	}

	cmd.Flags().StringVar(&endpoint, "endpoint", "", "GraphQL endpoint URL")
	cmd.Flags().StringVar(&query, "query", "", "GraphQL query string")
	cmd.Flags().StringVar(&variables, "variables", "", "GraphQL variables JSON")
	cmd.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "Action timeout")

	return cmd
}

func clientFromCmd(cmd *cobra.Command) (*client.Client, error) {
	base, err := cmd.Root().PersistentFlags().GetString("api")
	if err != nil {
		base = envOrDefault("VOLANT_API_BASE", "http://127.0.0.1:7777")
	}
	return client.New(base)
}
