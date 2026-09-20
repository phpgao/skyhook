//go:build !unix && !darwin && !linux

package storage

// OSDiskChecker fallback for non-unix platforms
type OSDiskChecker struct{}

func NewOSDiskChecker() DiskChecker {
	return &OSDiskChecker{}
}

func (c *OSDiskChecker) GetDiskUsage(path string) (DiskUsage, error) {
	return DiskUsage{
		Path:       path,
		TotalBytes: 100 * 1024 * 1024 * 1024,
		FreeBytes:  50 * 1024 * 1024 * 1024,
	}, nil
}

func (c *OSDiskChecker) CheckFreeSpace(path string, minFreeBytes uint64) error {
	return nil
}

func (c *OSDiskChecker) CheckTaskSpace(path string, requiredBytes uint64, minFreeBytes uint64) error {
	return nil
}
