package storage

import "fmt"

// MockDiskChecker provides a configurable disk checker for unit testing
type MockDiskChecker struct {
	TotalBytes uint64
	FreeBytes  uint64
	Err        error
}

// NewMockDiskChecker creates a mock disk checker
func NewMockDiskChecker(total, free uint64) *MockDiskChecker {
	return &MockDiskChecker{
		TotalBytes: total,
		FreeBytes:  free,
	}
}

func (m *MockDiskChecker) GetDiskUsage(path string) (DiskUsage, error) {
	if m.Err != nil {
		return DiskUsage{}, m.Err
	}
	return DiskUsage{
		Path:       path,
		TotalBytes: m.TotalBytes,
		FreeBytes:  m.FreeBytes,
		UsedBytes:  m.TotalBytes - m.FreeBytes,
	}, nil
}

func (m *MockDiskChecker) CheckFreeSpace(path string, minFreeBytes uint64) error {
	if m.Err != nil {
		return m.Err
	}
	if minFreeBytes == 0 {
		return nil
	}
	if m.FreeBytes < minFreeBytes {
		return fmt.Errorf("%w: available %s, minimum reserved %s on %q",
			ErrInsufficientDiskSpace, FormatBytes(m.FreeBytes), FormatBytes(minFreeBytes), path)
	}
	return nil
}

func (m *MockDiskChecker) CheckTaskSpace(path string, requiredBytes uint64, minFreeBytes uint64) error {
	if m.Err != nil {
		return m.Err
	}
	if minFreeBytes == 0 && requiredBytes == 0 {
		return nil
	}
	if m.FreeBytes < minFreeBytes {
		return fmt.Errorf("%w: available %s is below minimum reserve %s on %q",
			ErrInsufficientDiskSpace, FormatBytes(m.FreeBytes), FormatBytes(minFreeBytes), path)
	}
	if requiredBytes > 0 && m.FreeBytes < (requiredBytes+minFreeBytes) {
		allocatable := uint64(0)
		if m.FreeBytes > minFreeBytes {
			allocatable = m.FreeBytes - minFreeBytes
		}
		return fmt.Errorf("%w: task requires %s, but only %s can be safely allocated (free %s, reserved %s) on %q",
			ErrInsufficientDiskSpace, FormatBytes(requiredBytes),
			FormatBytes(allocatable),
			FormatBytes(m.FreeBytes), FormatBytes(minFreeBytes), path)
	}
	return nil
}
