package downloader

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"
)

// AdaptiveDownloader intelligently routes tasks between Aria2 and Native HTTP Downloader
type AdaptiveDownloader struct {
	aria2  *Aria2Client
	native *NativeHTTPDownloader

	aria2Online int32 // atomic boolean: 1 for online, 0 for offline
}

// NewAdaptiveDownloader creates an adaptive downloader that falls back gracefully
func NewAdaptiveDownloader(aria2 *Aria2Client, native *NativeHTTPDownloader) *AdaptiveDownloader {
	a := &AdaptiveDownloader{
		aria2:  aria2,
		native: native,
	}
	// Initial health check
	a.CheckAria2Health()
	return a
}

// CheckAria2Health probes the Aria2 RPC server
func (a *AdaptiveDownloader) CheckAria2Health() bool {
	if a.aria2 == nil {
		atomic.StoreInt32(&a.aria2Online, 0)
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := a.aria2.call(ctx, "aria2.getVersion", nil)
	if err == nil {
		atomic.StoreInt32(&a.aria2Online, 1)
		return true
	}

	atomic.StoreInt32(&a.aria2Online, 0)
	return false
}

// IsAria2Online returns true if Aria2 RPC is currently available
func (a *AdaptiveDownloader) IsAria2Online() bool {
	return atomic.LoadInt32(&a.aria2Online) == 1
}

func (a *AdaptiveDownloader) AddURI(ctx context.Context, uris []string, opts Options) (string, error) {
	if len(uris) == 0 {
		return "", fmt.Errorf("no uris provided")
	}

	firstURI := uris[0]
	isMagnet := strings.HasPrefix(strings.ToLower(firstURI), "magnet:")

	// Try checking health if currently thought to be offline
	if !a.IsAria2Online() {
		a.CheckAria2Health()
	}

	if a.IsAria2Online() {
		return a.aria2.AddURI(ctx, uris, opts)
	}

	// Aria2 is offline / missing
	if isMagnet {
		return "", fmt.Errorf("Aria2 is offline or not installed on this VPS; Magnet links require Aria2 to fetch DHT metadata")
	}

	// Fallback to pure Go Native HTTP Downloader for HTTP/HTTPS
	log.Printf("[Downloader] Aria2 is offline; seamlessly falling back to Native Go HTTP Downloader for: %s", firstURI)
	return a.native.AddURI(ctx, uris, opts)
}

func (a *AdaptiveDownloader) AddTorrent(ctx context.Context, base64Torrent string, opts Options) (string, error) {
	if !a.IsAria2Online() {
		a.CheckAria2Health()
	}

	if a.IsAria2Online() {
		return a.aria2.AddTorrent(ctx, base64Torrent, opts)
	}

	return "", fmt.Errorf("Aria2 is offline or not installed on this VPS; .torrent files require Aria2")
}

func (a *AdaptiveDownloader) TellStatus(ctx context.Context, gid string) (*DownloadStatus, error) {
	if strings.HasPrefix(gid, "native-") {
		return a.native.TellStatus(ctx, gid)
	}
	if a.aria2 != nil {
		return a.aria2.TellStatus(ctx, gid)
	}
	return nil, fmt.Errorf("no driver found for gid: %s", gid)
}

func (a *AdaptiveDownloader) ForceRemove(ctx context.Context, gid string) error {
	if strings.HasPrefix(gid, "native-") {
		return a.native.ForceRemove(ctx, gid)
	}
	if a.aria2 != nil {
		return a.aria2.ForceRemove(ctx, gid)
	}
	return nil
}

func (a *AdaptiveDownloader) ResolveTargetPath(status *DownloadStatus) string {
	if status == nil {
		return ""
	}
	if strings.HasPrefix(status.GID, "native-") {
		return a.native.ResolveTargetPath(status)
	}
	if a.aria2 != nil {
		return a.aria2.ResolveTargetPath(status)
	}
	return ""
}
