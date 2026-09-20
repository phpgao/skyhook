package downloader

import (
	"context"
)

// DownloadStatus represents the current state of a download in the engine
type DownloadStatus struct {
	GID             string     `json:"gid"`
	Status          string     `json:"status"` // "active", "waiting", "paused", "error", "complete", "removed"
	TotalLength     int64      `json:"total_length"`
	CompletedLength int64      `json:"completed_length"`
	UploadLength    int64      `json:"upload_length"`
	DownloadSpeed   int64      `json:"download_speed"`
	UploadSpeed     int64      `json:"upload_speed"`
	ErrorMessage    string     `json:"error_message"`
	Files           []FileInfo `json:"files"`
	Dir             string     `json:"dir"`
	TorrentName     string     `json:"torrent_name"`
}

// FileInfo represents an individual file in a download task
type FileInfo struct {
	Path      string `json:"path"`
	Length    int64  `json:"length"`
	Completed int64  `json:"completed"`
}

// Options contains optional settings for download tasks
type Options struct {
	DownloadLimit string `json:"download_limit,omitempty"` // e.g. "5M"
	UploadLimit   string `json:"upload_limit,omitempty"`   // e.g. "1M"
	Dir           string `json:"dir,omitempty"`
	SeedTime      string `json:"seed_time,omitempty"`      // e.g. "0"
	SeedRatio     string `json:"seed_ratio,omitempty"`     // e.g. "0.0"
	Trackers      string `json:"trackers,omitempty"`
}

// Downloader is the interface that download engines must satisfy
type Downloader interface {
	// Name returns the unique identifier of the downloader
	Name() string

	// TargetTypes returns the types of downloads this driver supports
	TargetTypes() []TargetType

	// Priority returns priority from 0 to 99 (99 is highest)
	Priority() int

	// IsAvailable checks if the underlying tool is installed or reachable
	IsAvailable(ctx context.Context) bool

	// AddURI submits one or more URIs (HTTP/HTTPS/FTP/Magnet/Video)
	AddURI(ctx context.Context, uris []string, opts Options) (string, error)

	// AddTorrent submits a base64 encoded .torrent file
	AddTorrent(ctx context.Context, base64Torrent string, opts Options) (string, error)

	// TellStatus returns current status for given GID
	TellStatus(ctx context.Context, gid string) (*DownloadStatus, error)

	// ForceRemove forcefully stops and removes a download task
	ForceRemove(ctx context.Context, gid string) error

	// ResolveTargetPath returns the final local path (file or directory) for a completed download
	ResolveTargetPath(status *DownloadStatus) string
}
