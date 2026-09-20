package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// NativeHTTPDownloader is a pure Go HTTP/HTTPS downloader with zero external dependencies
type NativeHTTPDownloader struct {
	downloadDir string
	httpClient  *http.Client

	mu      sync.RWMutex
	tasks   map[string]*DownloadStatus
	nextGID int64
}

// NewNativeHTTPDownloader creates a native Go HTTP downloader
func NewNativeHTTPDownloader(downloadDir string, client *http.Client) *NativeHTTPDownloader {
	if client == nil {
		client = &http.Client{}
	}
	return &NativeHTTPDownloader{
		downloadDir: downloadDir,
		httpClient:  client,
		tasks:       make(map[string]*DownloadStatus),
		nextGID:     5000,
	}
}

func (n *NativeHTTPDownloader) Name() string {
	return "native"
}

func (n *NativeHTTPDownloader) TargetTypes() []TargetType {
	return []TargetType{TargetHTTP}
}

func (n *NativeHTTPDownloader) Priority() int {
	return 1 // Lowest fallback priority
}

func (n *NativeHTTPDownloader) IsAvailable(ctx context.Context) bool {
	return true // Built-in Go client is always available
}

func (n *NativeHTTPDownloader) AddURI(ctx context.Context, uris []string, opts Options) (string, error) {
	if len(uris) == 0 {
		return "", fmt.Errorf("no uris provided")
	}

	targetURI := uris[0]
	parsed, err := url.Parse(targetURI)
	if err != nil {
		return "", fmt.Errorf("invalid uri: %w", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("native downloader only supports HTTP/HTTPS, got: %s (for magnet/bt, please start Aria2)", scheme)
	}

	gidNum := atomic.AddInt64(&n.nextGID, 1)
	gid := fmt.Sprintf("native-%d", gidNum)

	dir := n.downloadDir
	if opts.Dir != "" {
		dir = opts.Dir
	}
	_ = os.MkdirAll(dir, 0755)

	// Extract filename from URL path
	fileName := filepath.Base(parsed.Path)
	if fileName == "" || fileName == "/" || fileName == "." {
		fileName = fmt.Sprintf("download-%d.bin", gidNum)
	}

	destPath := filepath.Join(dir, fileName)

	status := &DownloadStatus{
		GID:    gid,
		Status: "active",
		Dir:    dir,
		Files: []FileInfo{
			{Path: destPath},
		},
	}

	n.mu.Lock()
	n.tasks[gid] = status
	n.mu.Unlock()

	// Run download in background
	go n.doDownload(gid, targetURI, destPath)

	return gid, nil
}

func (n *NativeHTTPDownloader) doDownload(gid, targetURI, destPath string) {
	req, err := http.NewRequest(http.MethodGet, targetURI, nil)
	if err != nil {
		n.markError(gid, err.Error())
		return
	}

	// Support HTTP Range断点续传 if partial file exists
	var existingSize int64
	if fi, err := os.Stat(destPath); err == nil && fi.Size() > 0 {
		existingSize = fi.Size()
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", existingSize))
	}

	resp, err := n.httpClient.Do(req)
	if err != nil {
		n.markError(gid, err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		n.markError(gid, fmt.Sprintf("server responded with status %d", resp.StatusCode))
		return
	}

	var out *os.File
	var written int64 = 0
	var totalLength int64 = resp.ContentLength

	if resp.StatusCode == http.StatusPartialContent && existingSize > 0 {
		// Server supports Range resume, append to partial file
		out, err = os.OpenFile(destPath, os.O_WRONLY|os.O_APPEND, 0644)
		written = existingSize
		if resp.ContentLength > 0 {
			totalLength = existingSize + resp.ContentLength
		}
	} else {
		// Normal download or server does not support Range
		out, err = os.Create(destPath)
		written = 0
		existingSize = 0
	}

	if err != nil {
		n.markError(gid, err.Error())
		return
	}
	defer out.Close()

	n.mu.Lock()
	if t, ok := n.tasks[gid]; ok {
		t.TotalLength = totalLength
		t.CompletedLength = written
	}
	n.mu.Unlock()

	// Copy data while updating completed length
	buf := make([]byte, 64*1024)
	for {
		nr, er := resp.Body.Read(buf)
		if nr > 0 {
			nw, ew := out.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
				n.mu.Lock()
				if t, ok := n.tasks[gid]; ok {
					t.CompletedLength = written
					if len(t.Files) > 0 {
						t.Files[0].Completed = written
					}
				}
				n.mu.Unlock()
			}
			if ew != nil {
				n.markError(gid, ew.Error())
				return
			}
		}
		if er != nil {
			if er != io.EOF {
				n.markError(gid, er.Error())
				return
			}
			break
		}
	}

	n.mu.Lock()
	if t, ok := n.tasks[gid]; ok {
		t.Status = "complete"
		t.CompletedLength = written
		if len(t.Files) > 0 {
			t.Files[0].Completed = written
			t.Files[0].Length = written
		}
	}
	n.mu.Unlock()
}

func (n *NativeHTTPDownloader) markError(gid, errMsg string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if t, ok := n.tasks[gid]; ok {
		t.Status = "error"
		t.ErrorMessage = errMsg
	}
}

func (n *NativeHTTPDownloader) AddTorrent(ctx context.Context, base64Torrent string, opts Options) (string, error) {
	return "", fmt.Errorf("native downloader does not support .torrent files (please run Aria2 on VPS for BT support)")
}

func (n *NativeHTTPDownloader) TellStatus(ctx context.Context, gid string) (*DownloadStatus, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	t, ok := n.tasks[gid]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", gid)
	}

	cpy := *t
	return &cpy, nil
}

func (n *NativeHTTPDownloader) ForceRemove(ctx context.Context, gid string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if t, ok := n.tasks[gid]; ok {
		t.Status = "removed"
		if len(t.Files) > 0 && t.Files[0].Path != "" {
			_ = os.Remove(t.Files[0].Path)
		}
	}
	return nil
}

func (n *NativeHTTPDownloader) ResolveTargetPath(status *DownloadStatus) string {
	if status == nil || len(status.Files) == 0 {
		return ""
	}
	return status.Files[0].Path
}
