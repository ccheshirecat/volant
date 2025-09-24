package standard

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/ccheshirecat/viper/internal/setup"
)

func newSetupCmd() *cobra.Command {
	var bridge string
	var subnet string
	var hostIP string
	var dryRun bool
	var runtimeDir string
	var logDir string
	var serviceFile string
	var kernelPath string
	var initramfsPath string

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure host networking and services for Viper",
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
			serverBinary := filepath.Join(filepath.Dir(exe), "viper-server")

			if kernelPath == "" {
				kernelPath = "build/artifacts/vmlinux-x86_64"
			}
			if initramfsPath == "" {
				initramfsPath = "build/artifacts/viper-initramfs.cpio.gz"
			}

			opts := setup.Options{
				BridgeName:    bridge,
				SubnetCIDR:    subnet,
				HostCIDR:      hostCIDR,
				DryRun:        dryRun,
				RuntimeDir:    runtimeDir,
				LogDir:        logDir,
				ServicePath:   serviceFile,
				BinaryPath:    serverBinary,
				KernelPath:    kernelPath,
				InitramfsPath: initramfsPath,
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

	cmd.Flags().StringVar(&bridge, "bridge", envOrDefault("VIPER_BRIDGE", "viperbr0"), "Name of the Linux bridge to create")
	cmd.Flags().StringVar(&subnet, "subnet", envOrDefault("VIPER_SUBNET", "192.168.127.0/24"), "Managed subnet CIDR")
	cmd.Flags().StringVar(&hostIP, "host-ip", envOrDefault("VIPER_HOST_IP", "192.168.127.1"), "Host IP address inside the bridge subnet")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print commands without executing them")
	cmd.Flags().StringVar(&runtimeDir, "runtime-dir", envOrDefault("VIPER_RUNTIME_DIR", "~/.viper/run"), "Runtime directory for VM sockets")
	cmd.Flags().StringVar(&logDir, "log-dir", envOrDefault("VIPER_LOG_DIR", "~/.viper/logs"), "Log directory for VM logs")
	cmd.Flags().StringVar(&serviceFile, "service-file", "/etc/systemd/system/viper-server.service", "Path to write systemd service unit (empty to skip)")
	cmd.Flags().StringVar(&kernelPath, "kernel", envOrDefault("VIPER_KERNEL", ""), "Path to kernel image for service (default: build/artifacts/vmlinux-x86_64)")
	cmd.Flags().StringVar(&initramfsPath, "initramfs", envOrDefault("VIPER_INITRAMFS", ""), "Path to initramfs image for service (default: build/artifacts/viper-initramfs.cpio.gz)")

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
