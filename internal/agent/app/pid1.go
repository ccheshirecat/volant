package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ccheshirecat/volant/internal/pluginspec"
	"golang.org/x/sys/unix"
)

const (
	rootMountPoint   = "/mnt/volant-root"
	oldRootPivotName = ".pivot-old"
)

var bootstrapOnce sync.Once

func (a *App) bootstrapPID1() error {
	var bootstrapErr error
	bootstrapOnce.Do(func() {
		if os.Getpid() != 1 {
			a.log.Printf("pid=%d; skipping pid1 bootstrap", os.Getpid())
			return
		}
		bootstrapErr = a.bootstrapPID1Inner()
	})
	return bootstrapErr
}

func (a *App) bootstrapPID1Inner() error {
	a.log.Printf("pid1 bootstrap starting")

	if err := mountInitial(); err != nil {
		return fmt.Errorf("mount initial filesystems: %w", err)
	}

	device := resolveRootfsDevice()
	if device == "" {
		return errors.New("rootfs device not specified")
	}
	if !strings.HasPrefix(device, "/dev/") {
		device = "/dev/" + device
	}

	fsType := resolveRootfsFSType()

	if err := waitForDevice(device, 10*time.Second); err != nil {
		return err
	}

	if err := mountRootfs(device, fsType); err != nil {
		return err
	}

	if err := copySelfToRoot(); err != nil {
		a.log.Printf("pid1 bootstrap warning: copy self failed: %v", err)
	}

	if err := pivotRoot(); err != nil {
		return err
	}

	if err := mountEssential(); err != nil {
		return fmt.Errorf("mount essentials: %w", err)
	}

	go reapZombies()
	go handleSignals(a)

	a.log.Printf("pid1 bootstrap complete")
	return nil
}

func mountInitial() error {
	toMount := []struct {
		source string
		target string
		fs     string
	}{
		{"proc", "/proc", "proc"},
		{"sysfs", "/sys", "sysfs"},
		{"devtmpfs", "/dev", "devtmpfs"},
	}
	for _, m := range toMount {
		if err := os.MkdirAll(m.target, 0o755); err != nil {
			return err
		}
		if err := unix.Mount(m.source, m.target, m.fs, 0, ""); err != nil && !errors.Is(err, unix.EBUSY) {
			return fmt.Errorf("mount %s on %s: %w", m.source, m.target, err)
		}
	}
	return nil
}

func resolveRootfsDevice() string {
	if value := cmdlineValue(pluginspec.RootFSDeviceKey); value != "" {
		return value
	}
	for _, candidate := range []string{"vda", "vdb", "sda", "sdb"} {
		if _, err := os.Stat("/dev/" + candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func resolveRootfsFSType() string {
	if value := cmdlineValue(pluginspec.RootFSFSTypeKey); value != "" {
		return value
	}
	return "ext4"
}

func cmdlineValue(key string) string {
	f, err := os.Open("/proc/cmdline")
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		for _, field := range strings.Fields(scanner.Text()) {
			if strings.HasPrefix(field, key+"=") {
				return strings.TrimPrefix(field, key+"=")
			}
		}
	}
	return ""
}

func waitForDevice(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("device %s not found", path)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func mountRootfs(device, fsType string) error {
	if err := os.MkdirAll(rootMountPoint, 0o755); err != nil {
		return err
	}
	if err := unix.Mount(device, rootMountPoint, fsType, unix.MS_RELATIME, ""); err != nil {
		return fmt.Errorf("mount rootfs %s on %s: %w", device, rootMountPoint, err)
	}
	return nil
}

func copySelfToRoot() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	src, err := os.Open(self)
	if err != nil {
		return err
	}
	defer src.Close()

	destPath := filepath.Join(rootMountPoint, "sbin", "init")
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	tmpPath := destPath + ".tmp"
	dest, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dest, src); err != nil {
		dest.Close()
		return err
	}
	if err := dest.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, destPath)
}

func pivotRoot() error {
	if err := os.MkdirAll(filepath.Join(rootMountPoint, oldRootPivotName), 0o755); err != nil {
		return err
	}
	if err := os.Chdir(rootMountPoint); err != nil {
		return err
	}
	if err := unix.PivotRoot(".", filepath.Join(".", oldRootPivotName)); err != nil {
		return fmt.Errorf("pivot_root: %w", err)
	}
	if err := os.Chdir("/"); err != nil {
		return err
	}
	oldRoot := filepath.Join("/", oldRootPivotName)
	if err := unix.Unmount(oldRoot, unix.MNT_DETACH); err != nil {
		return fmt.Errorf("unmount old root: %w", err)
	}
	return os.RemoveAll(oldRoot)
}

func mountEssential() error {
	mounts := []struct {
		source string
		target string
		fs     string
		flags  uintptr
		data   string
	}{
		{"proc", "/proc", "proc", 0, ""},
		{"sysfs", "/sys", "sysfs", 0, ""},
		{"devtmpfs", "/dev", "devtmpfs", 0, "mode=0755"},
	}
	for _, m := range mounts {
		if err := os.MkdirAll(m.target, 0o755); err != nil {
			return err
		}
		if err := unix.Mount(m.source, m.target, m.fs, m.flags, m.data); err != nil && !errors.Is(err, unix.EBUSY) {
			return fmt.Errorf("mount %s on %s: %w", m.source, m.target, err)
		}
	}
	return nil
}

func reapZombies() {
	for {
		_, _ = syscall.Wait4(-1, nil, 0, nil)
	}
}

func handleSignals(a *App) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	a.log.Printf("received signal %s, powering off", sig)
	_ = syscall.Reboot(syscall.LINUX_REBOOT_CMD_POWER_OFF)
}
