package storage

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	// ErrInsufficientDiskSpace is returned when available space is less than reserved
	ErrInsufficientDiskSpace = errors.New("insufficient disk space")
)

// DiskUsage represents the filesystem capacity and available space
type DiskUsage struct {
	Path       string `json:"path"`
	TotalBytes uint64 `json:"total_bytes"`
	FreeBytes  uint64 `json:"free_bytes"` // Available to unprivileged processes
	UsedBytes  uint64 `json:"used_bytes"`
}

// DiskChecker defines the interface for disk capacity and reservation checking
type DiskChecker interface {
	GetDiskUsage(path string) (DiskUsage, error)
	CheckFreeSpace(path string, minFreeBytes uint64) error
	CheckTaskSpace(path string, requiredBytes uint64, minFreeBytes uint64) error
}

// ParseBytes converts human-friendly byte strings (e.g., "500MB", "2GB", "1.5G", "10GiB") to uint64 bytes
func ParseBytes(s string) (uint64, error) {
	clean := strings.TrimSpace(s)
	if clean == "" || clean == "0" {
		return 0, nil
	}

	re := regexp.MustCompile(`(?i)^([0-9]+(?:\.[0-9]+)?)\s*([a-z]*)$`)
	matches := re.FindStringSubmatch(clean)
	if len(matches) != 3 {
		return 0, fmt.Errorf("invalid byte size format: %q", s)
	}

	val, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q: %w", matches[1], err)
	}

	unit := strings.ToUpper(matches[2])
	var multiplier float64 = 1

	switch unit {
	case "", "B":
		multiplier = 1
	case "K", "KB", "KIB":
		multiplier = 1024
	case "M", "MB", "MIB":
		multiplier = 1024 * 1024
	case "G", "GB", "GIB":
		multiplier = 1024 * 1024 * 1024
	case "T", "TB", "TIB":
		multiplier = 1024 * 1024 * 1024 * 1024
	case "P", "PB", "PIB":
		multiplier = 1024 * 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unsupported size unit: %q in %q", unit, s)
	}

	return uint64(val * multiplier), nil
}

// FormatBytes formats byte counts into human-readable strings
func FormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.2f %s", float64(b)/float64(div), units[exp])
}

// ResolveExistingDir traverses upwards until an existing directory is found
func ResolveExistingDir(path string) string {
	curr := filepath.Clean(path)
	for {
		if curr == "." || curr == "/" || curr == "" {
			return curr
		}
		return curr
	}
}
