package standard

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/ccheshirecat/volant/internal/setup"
)

func newSetupCmd() *cobra.Command {
	var bridge string
	var subnet string
	var hostIP string
	var dryRun bool
	var runtimeDir string
	var logDir string
	var serviceFile string
    var workDir string
    var kernelPath string

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure host networking and services for Volant",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
			defer cancel()

			if subnet == "" {
				subnet = "192.168.127.0/24"
			}
			if hostIP == "" {
				hostIP = "192.168.127.1"
			}

			hostCIDR, err := hostCIDRFrom(subnet, hostIP)
			if err != nil {
				return err
			}

			exe, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolve executable: %w", err)
			}
			serverBinary := filepath.Join(filepath.Dir(exe), "volantd")

			opts := setup.Options{
				BridgeName:  bridge,
				SubnetCIDR:  subnet,
				HostCIDR:    hostCIDR,
				DryRun:      dryRun,
				RuntimeDir:  runtimeDir,
				LogDir:      logDir,
				ServicePath: serviceFile,
				BinaryPath:  serverBinary,
				WorkDir:     workDir,
				KernelPath:  kernelPath,
			}

			res, err := setup.Run(ctx, opts)
			if err != nil {
				return err
			}
			if res != nil && len(res.Commands) > 0 {
				out := cmd.OutOrStdout()
				fmt.Fprintln(out, "Commands executed:")
				for _, line := range res.Commands {
					fmt.Fprintf(out, "  %s\n", line)
				}
			}
			if dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "Dry run complete. Re-run without --dry-run as root to apply changes.")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Setup completed successfully.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&bridge, "bridge", envOrDefault("VOLANT_BRIDGE", "vbr0"), "Name of the Linux bridge to create")
	cmd.Flags().StringVar(&subnet, "subnet", envOrDefault("VOLANT_SUBNET", "192.168.127.0/24"), "Managed subnet CIDR")
	cmd.Flags().StringVar(&hostIP, "host-ip", envOrDefault("volant_HOST_IP", "192.168.127.1"), "Host IP address inside the bridge subnet")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print commands without executing them")
	cmd.Flags().StringVar(&runtimeDir, "runtime-dir", envOrDefault("VOLANT_RUNTIME_DIR", "~/.volant/run"), "Runtime directory for VM sockets")
	cmd.Flags().StringVar(&logDir, "log-dir", envOrDefault("VOLANT_LOG_DIR", "~/.volant/logs"), "Log directory for VM logs")
	cmd.Flags().StringVar(&serviceFile, "service-file", "/etc/systemd/system/volantd.service", "Path to write systemd service unit (empty to skip)")
	cmd.Flags().StringVar(&workDir, "work-dir", envOrDefault("VOLANT_WORK_DIR", "/var/lib/volant"), "Working directory for volantd (systemd WorkingDirectory)")
	cmd.Flags().StringVar(&kernelPath, "kernel", envOrDefault("VOLANT_KERNEL", "/var/lib/volant/kernel/bzImage"), "Kernel image path for volantd (sets VOLANT_KERNEL)")

	return cmd
}

func hostCIDRFrom(subnet, host string) (string, error) {
	_, network, err := net.ParseCIDR(subnet)
	if err != nil {
		return "", fmt.Errorf("invalid subnet %s: %w", subnet, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "", fmt.Errorf("invalid host ip %s", host)
	}
	mask, _ := network.Mask.Size()
	return fmt.Sprintf("%s/%d", ip.String(), mask), nil
}
