package setup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Options controls the behaviour of the setup routine.
type Options struct {
	BridgeName  string
	SubnetCIDR  string
	HostCIDR    string
	DryRun      bool
	RuntimeDir  string
	LogDir      string
	ServicePath string
	BinaryPath  string
    // BZImagePath points to the bzImage kernel used for rootfs-based boot.
    // Example: /var/lib/volant/kernel/bzImage
    BZImagePath string
    // VMLinuxPath points to the vmlinux kernel used for initramfs-based boot.
    // Example: /var/lib/volant/kernel/vmlinux
    VMLinuxPath string
	// WorkDir is the WorkingDirectory for the volantd systemd unit.
	// Example: /var/lib/volant
	WorkDir string
}

// Result collects output and executed commands.
type Result struct {
	Commands []string
}

// Run performs host configuration for the VOLANT environment.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if opts.BridgeName == "" {
		opts.BridgeName = "vbr0"
	}
	if opts.SubnetCIDR == "" {
		opts.SubnetCIDR = "192.168.127.0/24"
	}
	if opts.HostCIDR == "" {
		opts.HostCIDR = "192.168.127.1/24"
	}
	if opts.RuntimeDir == "" {
		opts.RuntimeDir = "~/.volant/run"
	}
	if opts.LogDir == "" {
		opts.LogDir = "~/.volant/logs"
	}
	if strings.TrimSpace(opts.WorkDir) == "" {
		opts.WorkDir = "/var/lib/volant"
	}
    if strings.TrimSpace(opts.BZImagePath) == "" {
        opts.BZImagePath = filepath.Join(opts.WorkDir, "kernel", "bzImage")
    }
    if strings.TrimSpace(opts.VMLinuxPath) == "" {
        opts.VMLinuxPath = filepath.Join(opts.WorkDir, "kernel", "vmlinux")
    }

	res := &Result{}

	if !opts.DryRun {
		if os.Geteuid() != 0 {
			return nil, errors.New("volar setup must be run as root (use --dry-run to preview)")
		}
	}

	expand := func(path string) (string, error) {
		if path == "" {
			return "", nil
		}
		if strings.HasPrefix(path, "~") {
			home, err := os.UserHomeDir()
			if err != nil || home == "" {
				home = os.Getenv("HOME")
			}
			if home == "" && os.Geteuid() == 0 {
				home = "/root"
			}
			if home == "" {
				return "", fmt.Errorf("resolve home directory: %w", err)
			}
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		return abs, nil
	}

	runtimeDir, err := expand(opts.RuntimeDir)
	if err != nil {
		return nil, fmt.Errorf("expand runtime dir: %w", err)
	}
	logDir, err := expand(opts.LogDir)
	if err != nil {
		return nil, fmt.Errorf("expand log dir: %w", err)
	}
    binaryPath, err := expand(opts.BinaryPath)
    workDir, err := expand(opts.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("expand work dir: %w", err)
	}
    bzImagePath, err := expand(opts.BZImagePath)
    if err != nil {
        return nil, fmt.Errorf("expand bzImage path: %w", err)
    }
    vmlinuxPath, err := expand(opts.VMLinuxPath)
    if err != nil {
        return nil, fmt.Errorf("expand vmlinux path: %w", err)
    }
	if err != nil {
		return nil, fmt.Errorf("expand binary path: %w", err)
	}

	// Ensure directories exist.
	if err := ensureDir(runtimeDir, opts.DryRun, res); err != nil {
		return nil, err
	}
	if err := ensureDir(logDir, opts.DryRun, res); err != nil {
		return nil, err
	}
	// Ensure working directory (and kernel dir) exist
	if err := ensureDir(workDir, opts.DryRun, res); err != nil {
		return nil, err
	}
    if err := ensureDir(filepath.Dir(bzImagePath), opts.DryRun, res); err != nil {
        return nil, err
    }
    if err := ensureDir(filepath.Dir(vmlinuxPath), opts.DryRun, res); err != nil {
		return nil, err
	}

	// Ensure ip, iptables, cloud-hypervisor binaries exist.
	required := []string{"ip", "iptables", "cloud-hypervisor"}
	for _, bin := range required {
		if err := ensureBinary(bin); err != nil {
			return nil, err
		}
	}

	// Bridge setup.
	if err := runCommand(ctx, []string{"ip", "link", "add", opts.BridgeName, "type", "bridge"}, opts.DryRun, res, true); err != nil {
		return nil, err
	}
	if err := runCommand(ctx, []string{"ip", "addr", "replace", opts.HostCIDR, "dev", opts.BridgeName}, opts.DryRun, res, false); err != nil {
		return nil, err
	}
	if err := runCommand(ctx, []string{"ip", "link", "set", opts.BridgeName, "up"}, opts.DryRun, res, false); err != nil {
		return nil, err
	}

	// Enable IP forwarding.
	if err := writeFile("/proc/sys/net/ipv4/ip_forward", "1\n", opts.DryRun, res); err != nil {
		return nil, err
	}

	// iptables rules (idempotent).
	iptablesRules := [][]string{
		{"iptables", "-t", "nat", "-C", "POSTROUTING", "-s", opts.SubnetCIDR, "!", "-o", opts.BridgeName, "-j", "MASQUERADE"},
	}
	if err := ensureIptablesRule(ctx, iptablesRules[0], []string{"iptables", "-t", "nat", "-A", "POSTROUTING", "-s", opts.SubnetCIDR, "!", "-o", opts.BridgeName, "-j", "MASQUERADE"}, opts.DryRun, res); err != nil {
		return nil, err
	}
	forwardRules := [][]string{
		{"iptables", "-C", "FORWARD", "-i", opts.BridgeName, "-j", "ACCEPT"},
		{"iptables", "-C", "FORWARD", "-o", opts.BridgeName, "-j", "ACCEPT"},
	}
	for _, check := range forwardRules {
		add := append([]string{"iptables", "-A"}, check[2:]...)
		if err := ensureIptablesRule(ctx, check, add, opts.DryRun, res); err != nil {
			return nil, err
		}
	}

    if opts.ServicePath != "" {
        if err := writeServiceFile(binaryPath, opts, runtimeDir, logDir, workDir, bzImagePath, vmlinuxPath, opts.DryRun, res); err != nil {
			return nil, err
		}
		// Optionally enable and start the service automatically
		if !opts.DryRun {
			_ = runCommand(ctx, []string{"systemctl", "daemon-reload"}, false, res, false)
			_ = runCommand(ctx, []string{"systemctl", "enable", "--now", "volantd"}, false, res, true)
		} else {
			res.Commands = append(res.Commands, "systemctl daemon-reload")
			res.Commands = append(res.Commands, "systemctl enable --now volantd")
		}
	}

	return res, nil
}

func ensureBinary(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("required binary %s not found in PATH", name)
	}
	return nil
}

func ensureDir(path string, dryRun bool, res *Result) error {
	if path == "" {
		return errors.New("directory path cannot be empty")
	}
	if dryRun {
		res.Commands = append(res.Commands, fmt.Sprintf("mkdir -p %s", path))
		return nil
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create dir %s: %w", path, err)
	}
	return nil
}

func runCommand(ctx context.Context, args []string, dryRun bool, res *Result, ignoreErrors bool) error {
	res.Commands = append(res.Commands, strings.Join(args, " "))
	if dryRun {
		return nil
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	if err := cmd.Run(); err != nil {
		if ignoreErrors {
			return nil
		}
		return fmt.Errorf("run %v: %w", args, err)
	}
	return nil
}

func ensureIptablesRule(ctx context.Context, check, add []string, dryRun bool, res *Result) error {
	if dryRun {
		res.Commands = append(res.Commands, strings.Join(add, " "))
		return nil
	}
	cmd := exec.CommandContext(ctx, check[0], check[1:]...)
	if err := cmd.Run(); err != nil {
		cmdAdd := exec.CommandContext(ctx, add[0], add[1:]...)
		if err := cmdAdd.Run(); err != nil {
			return fmt.Errorf("add iptables rule: %w", err)
		}
		res.Commands = append(res.Commands, strings.Join(add, " "))
	}
	return nil
}

func writeFile(path, data string, dryRun bool, res *Result) error {
	res.Commands = append(res.Commands, fmt.Sprintf("echo '%s' > %s", strings.TrimSpace(data), path))
	if dryRun {
		return nil
	}
	return os.WriteFile(path, []byte(data), 0o644)
}

func writeServiceFile(binaryPath string, opts Options, runtimeDir, logDir, workDir, bzImagePath, vmlinuxPath string, dryRun bool, res *Result) error {
	if binaryPath == "" {
		return errors.New("server binary path required when writing service file")
	}

	logFile := filepath.Join(logDir, "volantd.log")
    service := fmt.Sprintf(`[Unit]
Description=VOLANT Control Plane
After=network.target

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=%s
Environment=VOLANT_BRIDGE=%s
Environment=VOLANT_SUBNET=%s
Environment=VOLANT_RUNTIME_DIR=%s
Environment=VOLANT_LOG_DIR=%s
Environment=VOLANT_KERNEL_BZIMAGE=%s
Environment=VOLANT_KERNEL_VMLINUX=%s
ExecStart=%s
Restart=always
RestartSec=5
StandardOutput=append:%s
StandardError=append:%s

[Install]
WantedBy=multi-user.target
`,
		workDir,
		opts.BridgeName,
		opts.SubnetCIDR,
		runtimeDir,
		logDir,
        bzImagePath,
        vmlinuxPath,
		binaryPath,
		logFile,
		logFile,
	)

    res.Commands = append(res.Commands, fmt.Sprintf("write service file %s", opts.ServicePath))
	if dryRun {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(opts.ServicePath), 0o755); err != nil {
		return fmt.Errorf("prepare service directory: %w", err)
	}
	f, err := os.Create(opts.ServicePath)
	if err != nil {
		return fmt.Errorf("create service file: %w", err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	if _, err := w.WriteString(service); err != nil {
		return fmt.Errorf("write service file: %w", err)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush service file: %w", err)
	}
	return nil
}
