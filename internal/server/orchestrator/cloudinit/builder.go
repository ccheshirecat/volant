// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

package cloudinit

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	diskfs "github.com/diskfs/go-diskfs"
	diskpkg "github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
)

const seedLabel = "CIDATA"

// SeedInput describes the documents used to build a NoCloud seed image.
type SeedInput struct {
	InstanceID    string
	Hostname      string
	UserData      string
	MetaData      string
	NetworkConfig string
}

// Build creates a cloud-init seed image at dest using either cloud-localds or genisoimage/mkisofs.
func Build(ctx context.Context, input SeedInput, dest string) error {
	if strings.TrimSpace(dest) == "" {
		return fmt.Errorf("cloudinit: destination path required")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("cloudinit: ensure destination directory: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "cloudinit-seed-")
	if err != nil {
		return fmt.Errorf("cloudinit: temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	userData := strings.TrimSpace(input.UserData)
	if userData == "" {
		userData = "#cloud-config\n"
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "user-data"), []byte(userData), 0o644); err != nil {
		return fmt.Errorf("cloudinit: write user-data: %w", err)
	}

	metaData := strings.TrimSpace(input.MetaData)
	if metaData == "" {
		instID := strings.TrimSpace(input.InstanceID)
		if instID == "" {
			instID = "volant-instance"
		}
		hostname := strings.TrimSpace(input.Hostname)
		if hostname == "" {
			hostname = instID
		}
		metaData = fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", instID, hostname)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "meta-data"), []byte(metaData), 0o644); err != nil {
		return fmt.Errorf("cloudinit: write meta-data: %w", err)
	}

	networkPath := ""
	if strings.TrimSpace(input.NetworkConfig) != "" {
		networkPath = filepath.Join(tmpDir, "network-config")
		if err := os.WriteFile(networkPath, []byte(input.NetworkConfig), 0o644); err != nil {
			return fmt.Errorf("cloudinit: write network-config: %w", err)
		}
	}

	if hasCommand("cloud-localds") {
		if err := runCloudLocalDS(ctx, dest, tmpDir, networkPath); err == nil {
			return nil
		}
	}
	files := map[string][]byte{
		"user-data": []byte(userData),
		"meta-data": []byte(metaData),
	}
	if strings.TrimSpace(input.NetworkConfig) != "" {
		files["network-config"] = []byte(input.NetworkConfig)
	}
	return buildVFAT(dest, files)
}

func runCloudLocalDS(ctx context.Context, dest, tmpDir, networkPath string) error {
	args := []string{}
	if networkPath != "" {
		args = append(args, "--network-config", networkPath)
	}
	args = append(args, dest, filepath.Join(tmpDir, "user-data"), filepath.Join(tmpDir, "meta-data"))
	cmd := exec.CommandContext(ctx, "cloud-localds", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func buildVFAT(dest string, files map[string][]byte) error {
	const imageSize = 64 * 1024 * 1024
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return err
	}
	disk, err := diskfs.Create(dest, imageSize, diskfs.SectorSize512)
	if err != nil {
		return fmt.Errorf("cloudinit: create disk image: %w", err)
	}
	fs, err := disk.CreateFilesystem(diskpkg.FilesystemSpec{
		Partition:   0,
		FSType:      filesystem.TypeFat32,
		VolumeLabel: seedLabel,
	})
	if err != nil {
		return fmt.Errorf("cloudinit: create filesystem: %w", err)
	}
	defer fs.Close()

	for name, data := range files {
		filePath := "/" + name
		handle, err := fs.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC)
		if err != nil {
			return fmt.Errorf("cloudinit: open %s: %w", name, err)
		}
		if _, err := handle.Write(data); err != nil {
			handle.Close()
			return fmt.Errorf("cloudinit: write %s: %w", name, err)
		}
		if err := handle.Close(); err != nil {
			return fmt.Errorf("cloudinit: close %s: %w", name, err)
		}
	}
	return nil
}
