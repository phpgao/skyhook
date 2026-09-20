package downloader

import (
	"context"
	"fmt"
	"sync"
)

// MockDownloader is an in-memory thread-safe implementation of Downloader for unit tests
type MockDownloader struct {
	mu           sync.Mutex
	Tasks        map[string]*DownloadStatus
	NextGID      int
	TargetPaths  map[string]string
	ForceRemoved map[string]bool
	FailOnAdd    bool
}

// NewMockDownloader creates a new mock downloader
func NewMockDownloader() *MockDownloader {
	return &MockDownloader{
		Tasks:        make(map[string]*DownloadStatus),
		TargetPaths:  make(map[string]string),
		ForceRemoved: make(map[string]bool),
		NextGID:      1000,
	}
}

func (m *MockDownloader) Name() string {
	return "mock"
}

func (m *MockDownloader) TargetTypes() []TargetType {
	return []TargetType{TargetHTTP, TargetBT, TargetMagnet, TargetVideo}
}

func (m *MockDownloader) Priority() int {
	return 99
}

func (m *MockDownloader) IsAvailable(ctx context.Context) bool {
	return true
}

func (m *MockDownloader) AddURI(ctx context.Context, uris []string, opts Options) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.FailOnAdd {
		return "", fmt.Errorf("mock error adding uri")
	}

	m.NextGID++
	gid := fmt.Sprintf("%d", m.NextGID)
	m.Tasks[gid] = &DownloadStatus{
		GID:    gid,
		Status: "active",
		Dir:    opts.Dir,
	}
	return gid, nil
}

func (m *MockDownloader) AddTorrent(ctx context.Context, base64Torrent string, opts Options) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.FailOnAdd {
		return "", fmt.Errorf("mock error adding torrent")
	}

	m.NextGID++
	gid := fmt.Sprintf("%d", m.NextGID)
	m.Tasks[gid] = &DownloadStatus{
		GID:    gid,
		Status: "active",
		Dir:    opts.Dir,
	}
	return gid, nil
}

func (m *MockDownloader) TellStatus(ctx context.Context, gid string) (*DownloadStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	status, ok := m.Tasks[gid]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", gid)
	}
	copied := *status
	return &copied, nil
}

func (m *MockDownloader) ForceRemove(ctx context.Context, gid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ForceRemoved[gid] = true
	if status, ok := m.Tasks[gid]; ok {
		status.Status = "removed"
	}
	return nil
}

func (m *MockDownloader) ResolveTargetPath(status *DownloadStatus) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if path, ok := m.TargetPaths[status.GID]; ok {
		return path
	}
	if len(status.Files) > 0 {
		return status.Files[0].Path
	}
	return ""
}

// SetTaskStatus helper to transition status in tests
func (m *MockDownloader) SetTaskStatus(gid, status string, files []FileInfo, targetPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if t, ok := m.Tasks[gid]; ok {
		t.Status = status
		t.Files = files
	}
	if targetPath != "" {
		m.TargetPaths[gid] = targetPath
	}
}

// IsForceRemoved returns whether ForceRemove was called for the given GID in a thread-safe manner
func (m *MockDownloader) IsForceRemoved(gid string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ForceRemoved[gid]
}

