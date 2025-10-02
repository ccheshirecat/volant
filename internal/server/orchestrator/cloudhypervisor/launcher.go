package cloudhypervisor

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ccheshirecat/volant/internal/server/orchestrator/runtime"
)

// Launcher knows how to boot Cloud Hypervisor microVMs.
type Launcher struct {
	Binary        string
    // Default kernel paths; launcher may pick based on spec
    BZImagePath   string
    VMLinuxPath   string
	RuntimeDir    string
	LogDir        string
	ConsoleDir    string
}

// New returns a configured Launcher.
func New(binary, bzImage, vmlinux, runtimeDir, logDir string) *Launcher {
	return &Launcher{
		Binary:        binary,
        BZImagePath:   bzImage,
        VMLinuxPath:   vmlinux,
		RuntimeDir:    runtimeDir,
		LogDir:        logDir,
	}
}

// Launch starts a Cloud Hypervisor process with the provided spec.
func (l *Launcher) Launch(ctx context.Context, spec runtime.LaunchSpec) (runtime.Instance, error) {
	if l.Binary == "" {
		return nil, fmt.Errorf("cloudhypervisor: binary path required")
	}
	if err := os.MkdirAll(l.RuntimeDir, 0o755); err != nil {
		return nil, fmt.Errorf("cloudhypervisor: ensure runtime dir: %w", err)
	}
	if l.LogDir == "" {
		l.LogDir = l.RuntimeDir
	}
	if err := os.MkdirAll(l.LogDir, 0o755); err != nil {
		return nil, fmt.Errorf("cloudhypervisor: ensure log dir: %w", err)
	}

	apiSocket := filepath.Join(l.RuntimeDir, fmt.Sprintf("%s.sock", spec.Name))
	_ = os.Remove(apiSocket)

    // Select kernel: explicit override > mode default (initramfs -> vmlinux, else bzImage)
    kernelSrc := strings.TrimSpace(spec.KernelOverride)
    if kernelSrc == "" {
        if strings.TrimSpace(spec.Initramfs) != "" && l.VMLinuxPath != "" {
            kernelSrc = l.VMLinuxPath
        } else {
            kernelSrc = l.BZImagePath
        }
    }
    if kernelSrc == "" {
        return nil, fmt.Errorf("cloudhypervisor: kernel path required")
    }

    // Preserve extension for readability
    ext := filepath.Ext(kernelSrc)
    if ext == "" {
        ext = ".vmlinux"
    }
    kernelCopy := filepath.Join(l.RuntimeDir, fmt.Sprintf("%s%s", spec.Name, ext))
    if err := copyFile(kernelSrc, kernelCopy); err != nil {
		return nil, fmt.Errorf("cloudhypervisor: stage kernel: %w", err)
	}

	var initramfsCopy string
    if strings.TrimSpace(spec.Initramfs) != "" {
        initramfsCopy = filepath.Join(l.RuntimeDir, fmt.Sprintf("%s.initramfs", spec.Name))
        if err := streamFile(ctx, spec.Initramfs, initramfsCopy, spec.InitramfsChecksum); err != nil {
			_ = os.Remove(kernelCopy)
			return nil, fmt.Errorf("cloudhypervisor: stage initramfs: %w", err)
		}
	}

	var rootfsPath string
	if spec.RootFS != "" {
		rootfsPath = filepath.Join(l.RuntimeDir, fmt.Sprintf("%s.rootfs", spec.Name))
		if err := streamFile(ctx, spec.RootFS, rootfsPath, spec.RootFSChecksum); err != nil {
			_ = os.Remove(kernelCopy)
			if initramfsCopy != "" {
				_ = os.Remove(initramfsCopy)
			}
			return nil, fmt.Errorf("cloudhypervisor: fetch rootfs: %w", err)
		}
	}

	logPath := filepath.Join(l.LogDir, fmt.Sprintf("%s.log", spec.Name))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		_ = os.Remove(kernelCopy)
		if initramfsCopy != "" {
			_ = os.Remove(initramfsCopy)
		}
		if rootfsPath != "" {
			_ = os.Remove(rootfsPath)
		}
		return nil, fmt.Errorf("cloudhypervisor: open log file: %w", err)
	}

	// Configure network based on whether tap device is provided
	// Empty tap device indicates vsock-only mode (no IP networking)
	var netArg string
	if spec.TapDevice != "" {
		netArg = fmt.Sprintf("tap=%s,mac=%s", spec.TapDevice, spec.MACAddress)
		if ip := strings.TrimSpace(spec.IPAddress); ip != "" {
			netArg = fmt.Sprintf("%s,ip=%s", netArg, ip)
		}
		if mask := strings.TrimSpace(spec.Netmask); mask != "" {
			netArg = fmt.Sprintf("%s,mask=%s", netArg, mask)
		}
	}
	if l.ConsoleDir == "" {
		l.ConsoleDir = l.RuntimeDir
	}
	if err := os.MkdirAll(l.ConsoleDir, 0o755); err != nil {
		return nil, fmt.Errorf("cloudhypervisor: ensure console dir: %w", err)
	}

	serialPath := spec.SerialSocket
	if serialPath == "" {
		serialPath = filepath.Join(l.ConsoleDir, fmt.Sprintf("%s.serial", spec.Name))
	}
	serialPath = filepath.Clean(serialPath)
	if err := os.MkdirAll(filepath.Dir(serialPath), 0o755); err != nil {
		return nil, fmt.Errorf("cloudhypervisor: ensure serial dir: %w", err)
	}
	if err := removeIfExists(serialPath); err != nil {
		return nil, fmt.Errorf("cloudhypervisor: prepare serial socket: %w", err)
	}
	serialPath, err = filepath.Abs(serialPath)
	if err != nil {
		return nil, fmt.Errorf("cloudhypervisor: resolve serial socket path: %w", err)
	}
	spec.SerialSocket = serialPath

	serialMode := fmt.Sprintf("socket=%s", spec.SerialSocket)

    args := []string{
		"--api-socket", fmt.Sprintf("path=%s", apiSocket),
		"--cpus", fmt.Sprintf("boot=%d", spec.CPUCores),
		"--memory", fmt.Sprintf("size=%dM", spec.MemoryMB),
		"--kernel", kernelCopy,
		"--serial", serialMode,
		"--console", "off",
	}
	if netArg != "" {
		// Bridged or DHCP mode: configure network interface
		args = append(args, "--net", netArg)
	} else {
		// Vsock-only mode: configure vsock device for host-guest communication
		// Use the allocated CID from the spec
		vsockArg := fmt.Sprintf("cid=%d", spec.VsockCID)
		args = append(args, "--vsock", vsockArg)
	}
    if initramfsCopy != "" {
		args = append(args, "--initramfs", initramfsCopy)
	}
	if rootfsPath != "" {
		args = append(args, "--disk", fmt.Sprintf("path=%s,readonly=false", rootfsPath))
	}
	for _, disk := range spec.Disks {
		path := strings.TrimSpace(disk.Path)
		if path == "" {
			continue
		}
		readonly := "false"
		if disk.Readonly {
			readonly = "true"
		}
		args = append(args, "--disk", fmt.Sprintf("path=%s,readonly=%s", path, readonly))
	}
	if spec.SeedDisk != nil {
		seedPath := strings.TrimSpace(spec.SeedDisk.Path)
		if seedPath != "" {
			readonly := "false"
			if spec.SeedDisk.Readonly {
				readonly = "true"
			}
			args = append(args, "--disk", fmt.Sprintf("path=%s,readonly=%s", seedPath, readonly))
		}
	}

	cmdline := spec.KernelCmdline
	if len(spec.Args) > 0 {
		appendix := make([]string, 0, len(spec.Args))
		for key, value := range spec.Args {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if strings.TrimSpace(value) == "" {
				appendix = append(appendix, key)
				continue
			}
			appendix = append(appendix, fmt.Sprintf("%s=%s", key, strings.TrimSpace(value)))
		}
		if len(appendix) > 0 {
			cmdline = strings.TrimSpace(cmdline + " " + strings.Join(appendix, " "))
		}
	}
	if spec.IPAddress != "" && spec.Netmask != "" && spec.Gateway != "" {
		hasIP := false
		for _, field := range strings.Fields(cmdline) {
			if strings.HasPrefix(field, "ip=") {
				hasIP = true
				break
			}
		}
		if !hasIP {
			cmdline = strings.TrimSpace(cmdline + " " + fmt.Sprintf("ip=%s::%s:%s::eth0", spec.IPAddress, spec.Gateway, spec.Netmask))
		}
	}
	args = append(args, "--cmdline", cmdline)

	select {
	case <-ctx.Done():
		logFile.Close()
		_ = os.Remove(kernelCopy)
		if initramfsCopy != "" {
			_ = os.Remove(initramfsCopy)
		}
		if rootfsPath != "" {
			_ = os.Remove(rootfsPath)
		}
		return nil, fmt.Errorf("cloudhypervisor: launch cancelled: %w", ctx.Err())
	default:
	}

	cmd := exec.CommandContext(ctx, l.Binary, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		_ = os.Remove(kernelCopy)
		if initramfsCopy != "" {
			_ = os.Remove(initramfsCopy)
		}
		if rootfsPath != "" {
			_ = os.Remove(rootfsPath)
		}
		return nil, fmt.Errorf("cloudhypervisor: start: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		done <- err
		close(done)
	}()

	return &instance{
		name:          spec.Name,
		cmd:           cmd,
		apiSocket:     apiSocket,
		serialPath:    serialPath,
		consolePath:   "", // Removed consolePath
		logFile:       logFile,
		done:          done,
		kernelPath:    kernelCopy,
		initramfsPath: initramfsCopy,
		rootfsPath:    rootfsPath,
	}, nil
}

type instance struct {
	name          string
	cmd           *exec.Cmd
	apiSocket     string
	serialPath    string
	consolePath   string
	logFile       *os.File
	done          <-chan error
	kernelPath    string
	initramfsPath string
	rootfsPath    string
}

func (i *instance) Name() string          { return i.name }
func (i *instance) PID() int              { return i.cmd.Process.Pid }
func (i *instance) APISocketPath() string { return i.apiSocket }
func (i *instance) Wait() <-chan error    { return i.done }

func (i *instance) Stop(ctx context.Context) error {
	defer i.logFile.Close()
	stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if i.cmd.Process == nil {
		return nil
	}

	if err := i.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("cloudhypervisor: signal term: %w", err)
	}

	select {
	case err, ok := <-i.done:
		if ok && err != nil {
			_ = os.Remove(i.apiSocket)
			return fmt.Errorf("cloudhypervisor: wait: %w", err)
		}
	case <-stopCtx.Done():
		_ = i.cmd.Process.Signal(syscall.SIGKILL)
		if err, ok := <-i.done; ok && err != nil {
			_ = os.Remove(i.apiSocket)
			return fmt.Errorf("cloudhypervisor: wait after kill: %w", err)
		}
	}

	_ = os.Remove(i.apiSocket)
	i.cleanupArtifacts()
	return nil
}

func (i *instance) cleanupArtifacts() {
	if i.kernelPath != "" {
		_ = os.Remove(i.kernelPath)
	}
	if i.initramfsPath != "" {
		_ = os.Remove(i.initramfsPath)
	}
	if i.rootfsPath != "" {
		_ = os.Remove(i.rootfsPath)
	}
	if i.serialPath != "" {
		_ = os.Remove(i.serialPath)
	}
	if i.consolePath != "" {
		_ = os.Remove(i.consolePath)
	}
}

func removeIfExists(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}

func touchFile(path string) error {
	if path == "" {
		return fmt.Errorf("touch: empty path")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	dest, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer dest.Close()

	if _, err := io.Copy(dest, source); err != nil {
		return err
	}
	return nil
}

func streamFile(ctx context.Context, src, dst, checksum string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	var reader io.ReadCloser
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode >= 300 {
			resp.Body.Close()
			return fmt.Errorf("download %s: status %s", src, resp.Status)
		}
		reader = resp.Body
	} else {
		reader, err = os.Open(src)
		if err != nil {
			return err
		}
	}
	defer reader.Close()

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, hasher), reader); err != nil {
		return err
	}

	if checksum != "" {
		expected := strings.TrimPrefix(strings.TrimSpace(checksum), "sha256:")
		actual := fmt.Sprintf("%x", hasher.Sum(nil))
		if !strings.EqualFold(expected, actual) {
			return fmt.Errorf("checksum mismatch: expected %s got %s", expected, actual)
		}
	}
	return nil
}

var _ runtime.Launcher = (*Launcher)(nil)
var _ runtime.Instance = (*instance)(nil)
