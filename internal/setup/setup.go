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
	BridgeName    string
	SubnetCIDR    string
	HostCIDR      string
	DryRun        bool
	RuntimeDir    string
	LogDir        string
	ServicePath   string
	BinaryPath    string
	KernelPath    string
	InitramfsPath string
}

// Result collects output and executed commands.
type Result struct {
	Commands []string
}

// Run performs host configuration for the Viper environment.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if opts.BridgeName == "" {
		opts.BridgeName = "viperbr0"
	}
	if opts.SubnetCIDR == "" {
		opts.SubnetCIDR = "192.168.127.0/24"
	}
	if opts.HostCIDR == "" {
		opts.HostCIDR = "192.168.127.1/24"
	}
	if opts.RuntimeDir == "" {
		opts.RuntimeDir = "~/.viper/run"
	}
	if opts.LogDir == "" {
		opts.LogDir = "~/.viper/logs"
	}

	res := &Result{}

	if !opts.DryRun {
		if os.Geteuid() != 0 {
			return nil, errors.New("viper setup must be run as root (use --dry-run to preview)")
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
	kernelPath, err := expand(opts.KernelPath)
	if err != nil {
		return nil, fmt.Errorf("expand kernel path: %w", err)
	}
	initramfsPath, err := expand(opts.InitramfsPath)
	if err != nil {
		return nil, fmt.Errorf("expand initramfs path: %w", err)
	}
	binaryPath, err := expand(opts.BinaryPath)
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
		if err := writeServiceFile(binaryPath, kernelPath, initramfsPath, opts, runtimeDir, logDir, opts.DryRun, res); err != nil {
			return nil, err
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

func writeServiceFile(binaryPath, kernelPath, initramfsPath string, opts Options, runtimeDir, logDir string, dryRun bool, res *Result) error {
	if binaryPath == "" {
		return errors.New("server binary path required when writing service file")
	}
	if kernelPath == "" || initramfsPath == "" {
		return errors.New("kernel and initramfs paths are required when writing service file")
	}
	if err := ensureFile(kernelPath); err != nil {
		return fmt.Errorf("kernel path invalid: %w", err)
	}
	if err := ensureFile(initramfsPath); err != nil {
		return fmt.Errorf("initramfs path invalid: %w", err)
	}

	logFile := filepath.Join(logDir, "viper-server.log")
	service := fmt.Sprintf(`[Unit]
Description=Viper Control Plane
After=network.target

[Service]
Type=simple
Environment=VIPER_KERNEL=%s
Environment=VIPER_INITRAMFS=%s
Environment=VIPER_BRIDGE=%s
Environment=VIPER_SUBNET=%s
Environment=VIPER_RUNTIME_DIR=%s
Environment=VIPER_LOG_DIR=%s
ExecStart=%s
Restart=always
RestartSec=5
StandardOutput=append:%s
StandardError=append:%s

[Install]
WantedBy=multi-user.target
`,
		kernelPath,
		initramfsPath,
		opts.BridgeName,
		opts.SubnetCIDR,
		runtimeDir,
		logDir,
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

func ensureFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("path %s is a directory", path)
	}
	return nil
}

// Err definitions for Setup.
var (
	ErrBinaryMissing = errors.New("required binary missing")
)
