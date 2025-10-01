package standard

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ccheshirecat/volant/internal/cli/client"
	"github.com/ccheshirecat/volant/internal/server/orchestrator/vmconfig"
	"golang.org/x/term"
)

func encodeAsJSON(out io.Writer, payload interface{}) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func DecodeBase64(data string) ([]byte, error) {
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
	cmd.AddCommand(newVMsConsoleCmd())
	cmd.AddCommand(newVMsNavigateCmd())
	cmd.AddCommand(newVMsScreenshotCmd())
	cmd.AddCommand(newVMsScrapeCmd())
	cmd.AddCommand(newVMsExecCmd())
	cmd.AddCommand(newVMsGraphQLCmd())
	cmd.AddCommand(newVMsStartCmd())
	cmd.AddCommand(newVMsStopCmd())
	cmd.AddCommand(newVMsRestartCmd())
	cmd.AddCommand(newVMsScaleCmd())
	cmd.AddCommand(newVMsConfigCmd())
	return cmd
}

func resolveConsoleSocket(ctx context.Context, api *client.Client, vmName, socketOverride string, useConsole bool) (string, string, error) {
	vm, err := api.GetVM(ctx, vmName)
	if err != nil {
		return "", "", err
	}
	if vm == nil {
		return "", "", fmt.Errorf("vm %s not found", vmName)
	}

	serialSocket := strings.TrimSpace(vm.SerialSocket)

	override := strings.TrimSpace(socketOverride)
	if override != "" {
		return override, "serial", nil
	}

	if strings.TrimSpace(serialSocket) == "" {
		return "", "", fmt.Errorf("no serial socket available")
	}
	return serialSocket, "serial", nil
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
			fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-10s %-10s %-15s %-20s %-6s %-6s\n", "NAME", "STATUS", "RUNTIME", "IP", "MAC", "CPU", "MEM")
			for _, vm := range vms {
				fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-10s %-10s %-15s %-20s %-6d %-6d\n", vm.Name, vm.Status, vm.Runtime, vm.IPAddress, vm.MACAddress, vm.CPUCores, vm.MemoryMB)
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
			fmt.Fprintf(cmd.OutOrStdout(), "Name: %s\nStatus: %s\nRuntime: %s\nIP: %s\nMAC: %s\nCPU: %d\nMemory: %d MB\n", vm.Name, vm.Status, vm.Runtime, vm.IPAddress, vm.MACAddress, vm.CPUCores, vm.MemoryMB)
			if vm.PID != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "PID: %d\n", *vm.PID)
			}
			if vm.KernelCmdline != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Kernel Cmdline: %s\n", vm.KernelCmdline)
			}
			if vm.SerialSocket != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Serial Socket: %s\n", vm.SerialSocket)
			}
			if vm.ConsoleSocket != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Console Socket: %s\n", vm.ConsoleSocket)
			}
			return nil
		},
	}
	return cmd
}

func newVMsCreateCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a microVM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtimeFlag, err := cmd.Flags().GetString("runtime")
			if err != nil {
				return err
			}
			cpuFlag, err := cmd.Flags().GetInt("cpu")
			if err != nil {
				return err
			}
			memFlag, err := cmd.Flags().GetInt("memory")
			if err != nil {
				return err
			}
			kernelFlag, err := cmd.Flags().GetString("kernel-cmdline")
			if err != nil {
				return err
			}
			pluginFlag, err := cmd.Flags().GetString("plugin")
			if err != nil {
				return err
			}
			apiHostFlag, err := cmd.Flags().GetString("api-host")
			if err != nil {
				return err
			}
			apiPortFlag, err := cmd.Flags().GetString("api-port")
			if err != nil {
				return err
			}
			configPath = strings.TrimSpace(configPath)

			var cfg *vmconfig.Config
			if configPath != "" {
				data, readErr := os.ReadFile(configPath)
				if readErr != nil {
					return readErr
				}
				var parsed vmconfig.Config
				if err := json.Unmarshal(data, &parsed); err != nil {
					var envelope struct {
						Config vmconfig.Config `json:"config"`
					}
					if err2 := json.Unmarshal(data, &envelope); err2 != nil {
						return fmt.Errorf("parse config file: %w", err)
					}
					parsed = envelope.Config
				}
				cfg = &parsed
			}

			pluginName := strings.TrimSpace(pluginFlag)
			if cfg != nil && strings.TrimSpace(cfg.Plugin) != "" {
				cfgPlugin := strings.TrimSpace(cfg.Plugin)
				if pluginName != "" && !strings.EqualFold(pluginName, cfgPlugin) {
					return fmt.Errorf("plugin mismatch between flag (%s) and config (%s)", pluginName, cfgPlugin)
				}
				pluginName = cfgPlugin
			}
			if pluginName == "" {
				return fmt.Errorf("plugin is required (flag or config file)")
			}

			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			manifest, err := api.DescribePlugin(ctx, pluginName)
			if err != nil {
				return err
			}
			manifest.Normalize()
			if manifest.Labels == nil {
				manifest.Labels = map[string]string{}
			}

			runtimeName := strings.TrimSpace(runtimeFlag)
			if cfg != nil && strings.TrimSpace(cfg.Runtime) != "" {
				runtimeName = strings.TrimSpace(cfg.Runtime)
			}
			if runtimeName == "" {
				runtimeName = manifest.Runtime
			}
			if strings.TrimSpace(runtimeName) == "" {
				runtimeName = manifest.Name
			}
			if strings.TrimSpace(runtimeName) == "" {
				return fmt.Errorf("plugin %s does not define a runtime", pluginName)
			}

			cpu := cpuFlag
			if cfg != nil && cfg.Resources.CPUCores > 0 {
				cpu = cfg.Resources.CPUCores
			}
			if cpu <= 0 {
				cpu = 2
			}

			mem := memFlag
			if cfg != nil && cfg.Resources.MemoryMB > 0 {
				mem = cfg.Resources.MemoryMB
			}
			if mem <= 0 {
				mem = 2048
			}

			kernelExtra := strings.TrimSpace(kernelFlag)
			if cfg != nil && strings.TrimSpace(cfg.KernelCmdline) != "" {
				kernelExtra = strings.TrimSpace(cfg.KernelCmdline)
			}

			apiHost := strings.TrimSpace(apiHostFlag)
			if cfg != nil && strings.TrimSpace(cfg.API.Host) != "" {
				apiHost = strings.TrimSpace(cfg.API.Host)
			}
			apiPort := strings.TrimSpace(apiPortFlag)
			if cfg != nil && strings.TrimSpace(cfg.API.Port) != "" {
				apiPort = strings.TrimSpace(cfg.API.Port)
			}

			req := client.CreateVMRequest{
				Name:          args[0],
				Plugin:        pluginName,
				Runtime:       runtimeName,
				CPUCores:      cpu,
				MemoryMB:      mem,
				KernelCmdline: kernelExtra,
				APIHost:       apiHost,
				APIPort:       apiPort,
			}
			if cfg != nil {
				cfgClone := cfg.Clone()
				cfgClone.Plugin = pluginName
				cfgClone.Runtime = runtimeName
				cfgClone.Resources = vmconfig.Resources{CPUCores: cpu, MemoryMB: mem}
				cfgClone.KernelCmdline = kernelExtra
				cfgClone.API = vmconfig.API{Host: apiHost, Port: apiPort}
				req.Config = &cfgClone
			}

			vm, err := api.CreateVM(ctx, req)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "VM %s created with IP %s\n", vm.Name, vm.IPAddress)
			return nil
		},
	}
	cmd.Flags().String("runtime", "", "Runtime type to launch (derived from plugin or config if omitted)")
	cmd.Flags().Int("cpu", 2, "Number of virtual CPU cores")
	cmd.Flags().Int("memory", 2048, "Memory (MB)")
	cmd.Flags().String("kernel-cmdline", "", "Additional kernel cmdline parameters")
	cmd.Flags().String("plugin", "", "Plugin name to use when creating the VM")
	cmd.Flags().StringVar(&configPath, "config", "", "Path to a VM config JSON file")
	cmd.Flags().String("api-host", "", "Override agent API host for the VM")
	cmd.Flags().String("api-port", "", "Override agent API port for the VM")
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

func newVMsStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start <name>",
		Short: "Start a stopped microVM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			vm, err := api.StartVM(ctx, args[0])
			if err != nil {
				return err
			}
			if vm.PID != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "VM %s started (PID %d)\n", vm.Name, *vm.PID)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "VM %s started\n", vm.Name)
			}
			return nil
		},
	}
	return cmd
}

func newVMsStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop a running microVM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			vm, err := api.StopVM(ctx, args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "VM %s stopped\n", vm.Name)
			return nil
		},
	}
	return cmd
}

func newVMsRestartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart <name>",
		Short: "Restart a microVM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
			defer cancel()

			vm, err := api.RestartVM(ctx, args[0])
			if err != nil {
				return err
			}
			if vm.PID != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "VM %s restarted (PID %d)\n", vm.Name, *vm.PID)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "VM %s restarted\n", vm.Name)
			}
			return nil
		},
	}
	return cmd
}

func newVMsScaleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scale <name>",
		Short: "Update microVM resources or scale deployments",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cpuVal, err := cmd.Flags().GetInt("cpu")
			if err != nil {
				return err
			}
			memVal, err := cmd.Flags().GetInt("memory")
			if err != nil {
				return err
			}
			restart, err := cmd.Flags().GetBool("restart")
			if err != nil {
				return err
			}
			replicasVal, err := cmd.Flags().GetInt("replicas")
			if err != nil {
				return err
			}
			if cpuVal <= 0 && memVal <= 0 && replicasVal < 0 {
				return fmt.Errorf("specify --cpu/--memory or --replicas to update")
			}

			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			if cpuVal > 0 || memVal > 0 {
				var resPatch vmconfig.ResourcesPatch
				if cpuVal > 0 {
					cpuCopy := cpuVal
					resPatch.CPUCores = &cpuCopy
				}
				if memVal > 0 {
					memCopy := memVal
					resPatch.MemoryMB = &memCopy
				}

				patch := vmconfig.Patch{Resources: &resPatch}
				updated, err := api.UpdateVMConfig(ctx, args[0], patch)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "VM %s updated: CPU=%d cores, Memory=%d MB (config version %d)\n",
					args[0], updated.Config.Resources.CPUCores, updated.Config.Resources.MemoryMB, updated.Version)
				if restart {
					restartCtx, cancelRestart := context.WithTimeout(cmd.Context(), 60*time.Second)
					defer cancelRestart()
					if _, err := api.RestartVM(restartCtx, args[0]); err != nil {
						return fmt.Errorf("config updated but restart failed: %w", err)
					}
					fmt.Fprintf(cmd.OutOrStdout(), "VM %s restarted to apply resource changes\n", args[0])
				}
			}

			if replicasVal >= 0 {
				deployment, err := api.ScaleDeployment(ctx, args[0], replicasVal)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Deployment %s scaled to %d replicas (ready %d)\n", deployment.Name, deployment.DesiredReplicas, deployment.ReadyReplicas)
			}
			return nil
		},
	}
	cmd.Flags().Int("cpu", -1, "Target number of virtual CPU cores")
	cmd.Flags().Int("memory", -1, "Target memory in MB")
	cmd.Flags().Bool("restart", false, "Restart the VM after updating resources")
	cmd.Flags().Int("replicas", -1, "Scale deployment replica count")
	return cmd
}

func newVMsConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and update VM configuration",
	}
	cmd.AddCommand(newVMsConfigGetCmd())
	cmd.AddCommand(newVMsConfigSetCmd())
	cmd.AddCommand(newVMsConfigHistoryCmd())
	return cmd
}

func newVMsConfigGetCmd() *cobra.Command {
	var outputPath string
	var raw bool
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Fetch the current VM configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()

			cfg, err := api.GetVMConfig(ctx, args[0])
			if err != nil {
				return err
			}
			var payload any
			if raw {
				payload = cfg
			} else {
				payload = cfg.Config
			}

			data, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return err
			}
			if outputPath != "" {
				return os.WriteFile(outputPath, data, 0o644)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
	cmd.Flags().StringVar(&outputPath, "output", "", "Write configuration to file instead of stdout")
	cmd.Flags().BoolVar(&raw, "raw", false, "Include metadata such as version and timestamps")
	return cmd
}

func newVMsConfigSetCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "set <name>",
		Short: "Replace VM configuration from a file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(filePath) == "" {
				return fmt.Errorf("--file is required")
			}
			data, err := os.ReadFile(filePath)
			if err != nil {
				return err
			}

			var cfg vmconfig.Config
			if err := json.Unmarshal(data, &cfg); err != nil {
				var envelope struct {
					Config vmconfig.Config `json:"config"`
				}
				if err2 := json.Unmarshal(data, &envelope); err2 != nil {
					return fmt.Errorf("parse config file: %w", err)
				}
				cfg = envelope.Config
			}
			payload, err := json.Marshal(cfg)
			if err != nil {
				return err
			}

			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			updated, err := api.UpdateVMConfigRaw(ctx, args[0], payload)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "VM %s configuration updated (version %d)\n", args[0], updated.Version)
			return nil
		},
	}
	cmd.Flags().StringVar(&filePath, "file", "", "Path to JSON configuration file")
	return cmd
}

func newVMsConfigHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history <name>",
		Short: "Show VM configuration history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			limit, err := cmd.Flags().GetInt("limit")
			if err != nil {
				return err
			}
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()
			history, err := api.GetVMConfigHistory(ctx, args[0], limit)
			if err != nil {
				return err
			}
			for _, entry := range history {
				fmt.Fprintf(cmd.OutOrStdout(), "Version %d	%s	CPU=%d	Memory=%d MB\n",
					entry.Version,
					entry.UpdatedAt.UTC().Format(time.RFC3339),
					entry.Config.Resources.CPUCores,
					entry.Config.Resources.MemoryMB,
				)
			}
			return nil
		},
	}
	cmd.Flags().Int("limit", 0, "Limit the number of history entries returned")
	return cmd
}

func newDeploymentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deployments",
		Short: "Manage VM deployments",
	}
	cmd.AddCommand(newDeploymentsListCmd())
	cmd.AddCommand(newDeploymentsCreateCmd())
	cmd.AddCommand(newDeploymentsGetCmd())
	cmd.AddCommand(newDeploymentsDeleteCmd())
	cmd.AddCommand(newDeploymentsScaleCmd())
	return cmd
}

func newDeploymentsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List deployments",
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()

			deployments, err := api.ListDeployments(ctx)
			if err != nil {
				return err
			}
			if len(deployments) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No deployments found")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-10s %-10s\n", "NAME", "DESIRED", "READY")
			for _, dep := range deployments {
				fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-10d %-10d\n", dep.Name, dep.DesiredReplicas, dep.ReadyReplicas)
			}
			return nil
		},
	}
	return cmd
}

func newDeploymentsCreateCmd() *cobra.Command {
	var configPath string
	var replicas int
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a deployment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(configPath) == "" {
				return fmt.Errorf("--config is required")
			}
			data, err := os.ReadFile(configPath)
			if err != nil {
				return err
			}
			var cfg vmconfig.Config
			if err := json.Unmarshal(data, &cfg); err != nil {
				var envelope struct {
					Config vmconfig.Config `json:"config"`
				}
				if err2 := json.Unmarshal(data, &envelope); err2 != nil {
					return fmt.Errorf("parse config file: %w", err)
				}
				cfg = envelope.Config
			}

			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			deployment, err := api.CreateDeployment(ctx, client.CreateDeploymentRequest{
				Name:     args[0],
				Replicas: replicas,
				Config:   cfg,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deployment %s created with %d replicas\n", deployment.Name, deployment.DesiredReplicas)
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "Path to deployment config JSON file")
	cmd.Flags().IntVar(&replicas, "replicas", 1, "Number of replicas to launch")
	return cmd
}

func newDeploymentsGetCmd() *cobra.Command {
	var outputPath string
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Show deployment details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()

			deployment, err := api.GetDeployment(ctx, args[0])
			if err != nil {
				return err
			}

			data, err := json.MarshalIndent(deployment, "", "  ")
			if err != nil {
				return err
			}
			if strings.TrimSpace(outputPath) != "" {
				return os.WriteFile(outputPath, data, 0o644)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
	cmd.Flags().StringVar(&outputPath, "output", "", "Write deployment details to file")
	return cmd
}

func newDeploymentsDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a deployment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			if err := api.DeleteDeployment(ctx, args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deployment %s deleted\n", args[0])
			return nil
		},
	}
	return cmd
}

func newDeploymentsScaleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scale <name> <replicas>",
		Short: "Scale deployment replicas",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			replicas, err := strconv.Atoi(args[1])
			if err != nil || replicas < 0 {
				return fmt.Errorf("replicas must be a non-negative integer")
			}
			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			deployment, err := api.ScaleDeployment(ctx, args[0], replicas)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deployment %s scaled to %d replicas (ready %d)\n", deployment.Name, deployment.DesiredReplicas, deployment.ReadyReplicas)
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

			data, decodeErr := DecodeBase64(resp.Data)
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

func newVMsConsoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "console <name>",
		Short: "Attach to a VM serial socket",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			socketPath, err := cmd.Flags().GetString("socket")
			if err != nil {
				return err
			}

			api, err := clientFromCmd(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()

			socketPath, _, err = resolveConsoleSocket(ctx, api, args[0], socketPath, false)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Connecting to serial socket: %s\n", socketPath)
			return attachUnixSocket(cmd, socketPath)
		},
	}
	cmd.Flags().String("socket", "", "Override socket path")
	return cmd
}

func attachUnixSocket(cmd *cobra.Command, socketPath string) error {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("connect unix socket: %w", err)
	}
	defer conn.Close()

	stdinFd := int(os.Stdin.Fd())
	if term.IsTerminal(stdinFd) {
		state, rawErr := term.MakeRaw(stdinFd)
		if rawErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to set raw mode: %v\n", rawErr)
		} else {
			defer term.Restore(stdinFd, state)
		}
	}

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
	defer signal.Stop(sigs)

	go func() {
		select {
		case <-ctx.Done():
		case <-sigs:
			cancel()
		}
	}()

	readerDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(conn, os.Stdin)
		cancel()
	}()
	go func() {
		_, _ = io.Copy(cmd.OutOrStdout(), conn)
		close(readerDone)
	}()

	select {
	case <-ctx.Done():
	case <-readerDone:
	}

	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func clientFromCmd(cmd *cobra.Command) (*client.Client, error) {
	base, err := cmd.Root().PersistentFlags().GetString("api")
	if err != nil {
		base = envOrDefault("VOLANT_API_BASE", "http://127.0.0.1:7777")
	}
	return client.New(base)
}
