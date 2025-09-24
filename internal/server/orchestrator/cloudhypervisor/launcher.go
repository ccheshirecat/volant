package cloudhypervisor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ccheshirecat/viper/internal/server/orchestrator/runtime"
)

// Launcher knows how to boot Cloud Hypervisor microVMs.
type Launcher struct {
	Binary        string
	KernelPath    string
	InitramfsPath string
	RuntimeDir    string
	LogDir        string
}

// New returns a configured Launcher.
func New(binary, kernel, initramfs, runtimeDir, logDir string) *Launcher {
	return &Launcher{
		Binary:        binary,
		KernelPath:    kernel,
		InitramfsPath: initramfs,
		RuntimeDir:    runtimeDir,
		LogDir:        logDir,
	}
}

// Launch starts a Cloud Hypervisor process with the provided spec.
func (l *Launcher) Launch(ctx context.Context, spec runtime.LaunchSpec) (runtime.Instance, error) {
	if l.Binary == "" {
		return nil, fmt.Errorf("cloudhypervisor: binary path required")
	}
	if l.KernelPath == "" {
		return nil, fmt.Errorf("cloudhypervisor: kernel path required")
	}
	if l.InitramfsPath == "" {
		return nil, fmt.Errorf("cloudhypervisor: initramfs path required")
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

	logPath := filepath.Join(l.LogDir, fmt.Sprintf("%s.log", spec.Name))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("cloudhypervisor: open log file: %w", err)
	}

	args := []string{
		"--api-socket", fmt.Sprintf("path=%s", apiSocket),
		"--cpus", fmt.Sprintf("boot=%d", spec.CPUCores),
		"--memory", fmt.Sprintf("size=%dM", spec.MemoryMB),
		"--kernel", l.KernelPath,
		"--initramfs", l.InitramfsPath,
		"--cmdline", spec.KernelCmdline,
		"--net", fmt.Sprintf("tap=%s,mac=%s", spec.TapDevice, spec.MACAddress),
		"--serial", "tty",
		"--console", "off",
	}

	cmd := exec.CommandContext(ctx, l.Binary, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("cloudhypervisor: start: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		done <- err
		close(done)
	}()

	return &instance{
		name:      spec.Name,
		cmd:       cmd,
		apiSocket: apiSocket,
		logFile:   logFile,
		done:      done,
	}, nil
}

type instance struct {
	name      string
	cmd       *exec.Cmd
	apiSocket string
	logFile   *os.File
	done      <-chan error
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
	return nil
}

var _ runtime.Launcher = (*Launcher)(nil)
var _ runtime.Instance = (*instance)(nil)
