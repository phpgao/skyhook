package downloader

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// CurlDownloader downloads files using the system curl binary
type CurlDownloader struct {
	downloadDir string
	priority    int

	mu      sync.RWMutex
	tasks   map[string]*DownloadStatus
	nextGID int64
}

// NewCurlDownloader creates a new curl downloader
func NewCurlDownloader(downloadDir string) *CurlDownloader {
	return &CurlDownloader{
		downloadDir: downloadDir,
		priority:    10, // Default priority 10 as specified
		tasks:       make(map[string]*DownloadStatus),
		nextGID:     7000,
	}
}

func (c *CurlDownloader) Name() string {
	return "curl"
}

func (c *CurlDownloader) TargetTypes() []TargetType {
	return []TargetType{TargetHTTP}
}

func (c *CurlDownloader) Priority() int {
	return c.priority
}

func (c *CurlDownloader) SetPriority(p int) {
	c.priority = p
}

func (c *CurlDownloader) IsAvailable(ctx context.Context) bool {
	_, err := exec.LookPath("curl")
	return err == nil
}

func (c *CurlDownloader) AddURI(ctx context.Context, uris []string, opts Options) (string, error) {
	if len(uris) == 0 {
		return "", fmt.Errorf("no uris provided")
	}

	targetURI := uris[0]
	parsed, err := url.Parse(targetURI)
	if err != nil {
		return "", fmt.Errorf("invalid uri: %w", err)
	}

	dir := c.downloadDir
	if opts.Dir != "" {
		dir = opts.Dir
	}
	_ = os.MkdirAll(dir, 0755)

	fileName := filepath.Base(parsed.Path)
	if fileName == "" || fileName == "/" || fileName == "." {
		fileName = "download.bin"
	}
	destPath := filepath.Join(dir, fileName)

	gidNum := atomic.AddInt64(&c.nextGID, 1)
	gid := fmt.Sprintf("curl-%d", gidNum)

	status := &DownloadStatus{
		GID:    gid,
		Status: "active",
		Dir:    dir,
		Files: []FileInfo{
			{Path: destPath},
		},
	}

	c.mu.Lock()
	c.tasks[gid] = status
	c.mu.Unlock()

	go func() {
		args := []string{"-L", "-C", "-", "-o", destPath, targetURI}
		if opts.DownloadLimit != "" && opts.DownloadLimit != "0" {
			args = append(args, "--limit-rate", opts.DownloadLimit)
		}

		cmd := exec.Command("curl", args...)
		out, err := cmd.CombinedOutput()

		c.mu.Lock()
		defer c.mu.Unlock()
		if t, ok := c.tasks[gid]; ok {
			if err != nil {
				t.Status = "error"
				t.ErrorMessage = fmt.Sprintf("%v: %s", err, string(out))
			} else {
				t.Status = "complete"
				if fi, err := os.Stat(destPath); err == nil {
					t.TotalLength = fi.Size()
					t.CompletedLength = fi.Size()
				}
			}
		}
	}()

	return gid, nil
}

func (c *CurlDownloader) AddTorrent(ctx context.Context, base64Torrent string, opts Options) (string, error) {
	return "", fmt.Errorf("curl does not support .torrent files")
}

func (c *CurlDownloader) TellStatus(ctx context.Context, gid string) (*DownloadStatus, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	t, ok := c.tasks[gid]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", gid)
	}
	cpy := *t
	return &cpy, nil
}

func (c *CurlDownloader) ForceRemove(ctx context.Context, gid string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if t, ok := c.tasks[gid]; ok {
		t.Status = "removed"
		if len(t.Files) > 0 {
			_ = os.Remove(t.Files[0].Path)
		}
	}
	return nil
}

func (c *CurlDownloader) ResolveTargetPath(status *DownloadStatus) string {
	if status == nil || len(status.Files) == 0 {
		return ""
	}
	return status.Files[0].Path
}
