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

// Aria2cCLIDownloader executes aria2c binary directly as a subprocess (fork, no RPC)
type Aria2cCLIDownloader struct {
	downloadDir string
	priority    int

	mu      sync.RWMutex
	tasks   map[string]*DownloadStatus
	nextGID int64
}

// NewAria2cCLIDownloader creates a new CLI fork aria2c downloader
func NewAria2cCLIDownloader(downloadDir string) *Aria2cCLIDownloader {
	return &Aria2cCLIDownloader{
		downloadDir: downloadDir,
		priority:    88, // Priority 88 as specified
		tasks:       make(map[string]*DownloadStatus),
		nextGID:     8000,
	}
}

func (a *Aria2cCLIDownloader) Name() string {
	return "aria2c_cli"
}

func (a *Aria2cCLIDownloader) TargetTypes() []TargetType {
	return []TargetType{TargetHTTP, TargetBT, TargetMagnet}
}

func (a *Aria2cCLIDownloader) Priority() int {
	return a.priority
}

func (a *Aria2cCLIDownloader) SetPriority(p int) {
	a.priority = p
}

func (a *Aria2cCLIDownloader) IsAvailable(ctx context.Context) bool {
	_, err := exec.LookPath("aria2c")
	return err == nil
}

func (a *Aria2cCLIDownloader) AddURI(ctx context.Context, uris []string, opts Options) (string, error) {
	if len(uris) == 0 {
		return "", fmt.Errorf("no uris provided")
	}

	targetURI := uris[0]
	dir := a.downloadDir
	if opts.Dir != "" {
		dir = opts.Dir
	}
	_ = os.MkdirAll(dir, 0755)

	parsed, _ := url.Parse(targetURI)
	fileName := "download.bin"
	if parsed != nil && parsed.Path != "" && parsed.Path != "/" {
		fileName = filepath.Base(parsed.Path)
	}
	destPath := filepath.Join(dir, fileName)

	gidNum := atomic.AddInt64(&a.nextGID, 1)
	gid := fmt.Sprintf("aria2cli-%d", gidNum)

	status := &DownloadStatus{
		GID:    gid,
		Status: "active",
		Dir:    dir,
		Files: []FileInfo{
			{Path: destPath},
		},
	}

	a.mu.Lock()
	a.tasks[gid] = status
	a.mu.Unlock()

	go func() {
		args := []string{
			"-x", "16",
			"-s", "16",
			"-d", dir,
		}
		if opts.DownloadLimit != "" && opts.DownloadLimit != "0" {
			args = append(args, fmt.Sprintf("--max-download-limit=%s", opts.DownloadLimit))
		}
		args = append(args, targetURI)

		cmd := exec.Command("aria2c", args...)
		out, err := cmd.CombinedOutput()

		a.mu.Lock()
		defer a.mu.Unlock()
		if t, ok := a.tasks[gid]; ok {
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

func (a *Aria2cCLIDownloader) AddTorrent(ctx context.Context, base64Torrent string, opts Options) (string, error) {
	return "", fmt.Errorf("aria2c cli fork for raw base64 torrent not implemented; use aria2_rpc or save .torrent first")
}

func (a *Aria2cCLIDownloader) TellStatus(ctx context.Context, gid string) (*DownloadStatus, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	t, ok := a.tasks[gid]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", gid)
	}
	cpy := *t
	return &cpy, nil
}

func (a *Aria2cCLIDownloader) ForceRemove(ctx context.Context, gid string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if t, ok := a.tasks[gid]; ok {
		t.Status = "removed"
		if len(t.Files) > 0 {
			_ = os.Remove(t.Files[0].Path)
		}
	}
	return nil
}

func (a *Aria2cCLIDownloader) ResolveTargetPath(status *DownloadStatus) string {
	if status == nil || len(status.Files) == 0 {
		return ""
	}
	return status.Files[0].Path
}
