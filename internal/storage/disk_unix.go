//go:build unix || darwin || linux

package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// OSDiskChecker performs real filesystem queries using unix.Statfs
type OSDiskChecker struct{}

// NewOSDiskChecker creates an OS-level disk checker
func NewOSDiskChecker() DiskChecker {
	return &OSDiskChecker{}
}

// GetDiskUsage retrieves filesystem size and free blocks
func (c *OSDiskChecker) GetDiskUsage(path string) (DiskUsage, error) {
	checkPath := path
	if checkPath == "" {
		checkPath = "."
	}

	// Traverse upwards if directory does not exist yet
	for {
		if fi, err := os.Stat(checkPath); err == nil && fi.IsDir() {
			break
		}
		parent := filepath.Dir(checkPath)
		if parent == checkPath {
			break
		}
		checkPath = parent
	}

	var stat unix.Statfs_t
	if err := unix.Statfs(checkPath, &stat); err != nil {
		return DiskUsage{}, fmt.Errorf("statfs failed on %q: %w", checkPath, err)
	}

	bsize := uint64(stat.Bsize)
	totalBytes := stat.Blocks * bsize
	freeBytes := stat.Bavail * bsize // Available to unprivileged processes
	usedBytes := uint64(0)
	if totalBytes >= (stat.Bfree * bsize) {
		usedBytes = totalBytes - (stat.Bfree * bsize)
	}

	return DiskUsage{
		Path:       checkPath,
		TotalBytes: totalBytes,
		FreeBytes:  freeBytes,
		UsedBytes:  usedBytes,
	}, nil
}

// CheckFreeSpace verifies that current free space is at least minFreeBytes
func (c *OSDiskChecker) CheckFreeSpace(path string, minFreeBytes uint64) error {
	if minFreeBytes == 0 {
		return nil
	}
	usage, err := c.GetDiskUsage(path)
	if err != nil {
		return fmt.Errorf("failed to query disk usage: %w", err)
	}
	if usage.FreeBytes < minFreeBytes {
		return fmt.Errorf("%w: available %s, minimum reserved %s on %q",
			ErrInsufficientDiskSpace, FormatBytes(usage.FreeBytes), FormatBytes(minFreeBytes), usage.Path)
	}
	return nil
}

// CheckTaskSpace verifies that adding requiredBytes will not violate the minFreeBytes reserve
func (c *OSDiskChecker) CheckTaskSpace(path string, requiredBytes uint64, minFreeBytes uint64) error {
	if minFreeBytes == 0 && requiredBytes == 0 {
		return nil
	}
	usage, err := c.GetDiskUsage(path)
	if err != nil {
		return fmt.Errorf("failed to query disk usage: %w", err)
	}
	if usage.FreeBytes < minFreeBytes {
		return fmt.Errorf("%w: available %s is already below minimum reserve %s on %q",
			ErrInsufficientDiskSpace, FormatBytes(usage.FreeBytes), FormatBytes(minFreeBytes), usage.Path)
	}
	if requiredBytes > 0 && usage.FreeBytes < (requiredBytes+minFreeBytes) {
		allocatable := uint64(0)
		if usage.FreeBytes > minFreeBytes {
			allocatable = usage.FreeBytes - minFreeBytes
		}
		return fmt.Errorf("%w: task requires %s, but only %s can be safely allocated (free %s, reserved %s) on %q",
			ErrInsufficientDiskSpace, FormatBytes(requiredBytes),
			FormatBytes(allocatable),
			FormatBytes(usage.FreeBytes), FormatBytes(minFreeBytes), usage.Path)
	}
	return nil
}
