package downloader

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// YtDlpDownloader downloads videos/audio using the yt-dlp binary (for YouTube, Bilibili, etc.)
type YtDlpDownloader struct {
	downloadDir string
	priority    int

	mu      sync.RWMutex
	tasks   map[string]*DownloadStatus
	nextGID int64
}

// NewYtDlpDownloader creates a new yt-dlp downloader
func NewYtDlpDownloader(downloadDir string) *YtDlpDownloader {
	return &YtDlpDownloader{
		downloadDir: downloadDir,
		priority:    10, // Priority 10 as specified by user
		tasks:       make(map[string]*DownloadStatus),
		nextGID:     9000,
	}
}

func (y *YtDlpDownloader) Name() string {
	return "yt-dlp"
}

func (y *YtDlpDownloader) TargetTypes() []TargetType {
	return []TargetType{TargetVideo}
}

func (y *YtDlpDownloader) Priority() int {
	return y.priority
}

func (y *YtDlpDownloader) SetPriority(p int) {
	y.priority = p
}

func (y *YtDlpDownloader) IsAvailable(ctx context.Context) bool {
	_, err := exec.LookPath("yt-dlp")
	return err == nil
}

func (y *YtDlpDownloader) AddURI(ctx context.Context, uris []string, opts Options) (string, error) {
	if len(uris) == 0 {
		return "", fmt.Errorf("no uris provided")
	}

	targetURI := uris[0]
	dir := y.downloadDir
	if opts.Dir != "" {
		dir = opts.Dir
	}
	_ = os.MkdirAll(dir, 0755)

	gidNum := atomic.AddInt64(&y.nextGID, 1)
	gid := fmt.Sprintf("ytdlp-%d", gidNum)

	status := &DownloadStatus{
		GID:    gid,
		Status: "active",
		Dir:    dir,
	}

	y.mu.Lock()
	y.tasks[gid] = status
	y.mu.Unlock()

	go func() {
		// Output template: %(title)s.%(ext)s
		args := []string{
			"-P", dir,
			"--no-playlist",
			"-o", "%(title)s.%(ext)s",
			"--print", "after_move:filepath",
			targetURI,
		}

		cmd := exec.Command("yt-dlp", args...)
		out, err := cmd.CombinedOutput()

		y.mu.Lock()
		defer y.mu.Unlock()
		if t, ok := y.tasks[gid]; ok {
			if err != nil {
				t.Status = "error"
				t.ErrorMessage = fmt.Sprintf("%v: %s", err, string(out))
			} else {
				t.Status = "complete"
				lines := strings.Split(strings.TrimSpace(string(out)), "\n")
				if len(lines) > 0 {
					lastLine := strings.TrimSpace(lines[len(lines)-1])
					if fi, err := os.Stat(lastLine); err == nil {
						t.Files = []FileInfo{{Path: lastLine, Length: fi.Size(), Completed: fi.Size()}}
						t.TotalLength = fi.Size()
						t.CompletedLength = fi.Size()
					}
				}
			}
		}
	}()

	return gid, nil
}

func (y *YtDlpDownloader) AddTorrent(ctx context.Context, base64Torrent string, opts Options) (string, error) {
	return "", fmt.Errorf("yt-dlp does not support torrents")
}

func (y *YtDlpDownloader) TellStatus(ctx context.Context, gid string) (*DownloadStatus, error) {
	y.mu.RLock()
	defer y.mu.RUnlock()

	t, ok := y.tasks[gid]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", gid)
	}
	cpy := *t
	return &cpy, nil
}

func (y *YtDlpDownloader) ForceRemove(ctx context.Context, gid string) error {
	y.mu.Lock()
	defer y.mu.Unlock()

	if t, ok := y.tasks[gid]; ok {
		t.Status = "removed"
		if len(t.Files) > 0 {
			_ = os.Remove(t.Files[0].Path)
		}
	}
	return nil
}

func (y *YtDlpDownloader) ResolveTargetPath(status *DownloadStatus) string {
	if status == nil {
		return ""
	}
	if len(status.Files) > 0 && status.Files[0].Path != "" {
		return status.Files[0].Path
	}
	// If output file was not captured via print, find newest file in dir
	matches, _ := filepath.Glob(filepath.Join(status.Dir, "*"))
	if len(matches) > 0 {
		return matches[len(matches)-1]
	}
	return ""
}
