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

// WgetDownloader downloads files using the system wget binary
type WgetDownloader struct {
	downloadDir string
	priority    int

	mu      sync.RWMutex
	tasks   map[string]*DownloadStatus
	nextGID int64
}

// NewWgetDownloader creates a new wget downloader
func NewWgetDownloader(downloadDir string) *WgetDownloader {
	return &WgetDownloader{
		downloadDir: downloadDir,
		priority:    19, // Default priority 19 as specified
		tasks:       make(map[string]*DownloadStatus),
		nextGID:     6000,
	}
}

func (w *WgetDownloader) Name() string {
	return "wget"
}

func (w *WgetDownloader) TargetTypes() []TargetType {
	return []TargetType{TargetHTTP}
}

func (w *WgetDownloader) Priority() int {
	return w.priority
}

func (w *WgetDownloader) SetPriority(p int) {
	w.priority = p
}

func (w *WgetDownloader) IsAvailable(ctx context.Context) bool {
	_, err := exec.LookPath("wget")
	return err == nil
}

func (w *WgetDownloader) AddURI(ctx context.Context, uris []string, opts Options) (string, error) {
	if len(uris) == 0 {
		return "", fmt.Errorf("no uris provided")
	}

	targetURI := uris[0]
	parsed, err := url.Parse(targetURI)
	if err != nil {
		return "", fmt.Errorf("invalid uri: %w", err)
	}

	dir := w.downloadDir
	if opts.Dir != "" {
		dir = opts.Dir
	}
	_ = os.MkdirAll(dir, 0755)

	fileName := filepath.Base(parsed.Path)
	if fileName == "" || fileName == "/" || fileName == "." {
		fileName = "download.bin"
	}
	destPath := filepath.Join(dir, fileName)

	gidNum := atomic.AddInt64(&w.nextGID, 1)
	gid := fmt.Sprintf("wget-%d", gidNum)

	status := &DownloadStatus{
		GID:    gid,
		Status: "active",
		Dir:    dir,
		Files: []FileInfo{
			{Path: destPath},
		},
	}

	w.mu.Lock()
	w.tasks[gid] = status
	w.mu.Unlock()

	go func() {
		args := []string{"-c", "-P", dir, targetURI}
		if opts.DownloadLimit != "" && opts.DownloadLimit != "0" {
			args = append(args, fmt.Sprintf("--limit-rate=%s", opts.DownloadLimit))
		}

		cmd := exec.Command("wget", args...)
		out, err := cmd.CombinedOutput()

		w.mu.Lock()
		defer w.mu.Unlock()
		if t, ok := w.tasks[gid]; ok {
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

func (w *WgetDownloader) AddTorrent(ctx context.Context, base64Torrent string, opts Options) (string, error) {
	return "", fmt.Errorf("wget does not support .torrent files")
}

func (w *WgetDownloader) TellStatus(ctx context.Context, gid string) (*DownloadStatus, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	t, ok := w.tasks[gid]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", gid)
	}
	cpy := *t
	return &cpy, nil
}

func (w *WgetDownloader) ForceRemove(ctx context.Context, gid string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if t, ok := w.tasks[gid]; ok {
		t.Status = "removed"
		if len(t.Files) > 0 {
			_ = os.Remove(t.Files[0].Path)
		}
	}
	return nil
}

func (w *WgetDownloader) ResolveTargetPath(status *DownloadStatus) string {
	if status == nil || len(status.Files) == 0 {
		return ""
	}
	return status.Files[0].Path
}
