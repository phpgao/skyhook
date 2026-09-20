package downloader

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Registry manages multiple downloaders and routes tasks based on target type and priority
type Registry struct {
	mu           sync.RWMutex
	downloaders  []Downloader
	videoDomains []string
	gidToDriver  map[string]Downloader
}

// NewRegistry creates a new Downloader registry
func NewRegistry(videoDomains []string) *Registry {
	return &Registry{
		videoDomains: videoDomains,
		gidToDriver:  make(map[string]Downloader),
	}
}

// Register adds a downloader driver to the registry
func (r *Registry) Register(d Downloader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.downloaders = append(r.downloaders, d)
}

// All returns all registered downloaders
func (r *Registry) All() []Downloader {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]Downloader, len(r.downloaders))
	copy(res, r.downloaders)
	return res
}

// SelectDownloader picks the best available downloader for the given input target
func (r *Registry) SelectDownloader(ctx context.Context, target string) (Downloader, TargetType, error) {
	targetType := DetectTargetType(target, r.videoDomains)

	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1. Filter candidates supporting this target type
	var candidates []Downloader
	for _, d := range r.downloaders {
		for _, t := range d.TargetTypes() {
			if t == targetType {
				candidates = append(candidates, d)
				break
			}
		}
	}

	if len(candidates) == 0 {
		return nil, targetType, fmt.Errorf("no downloader registered for target type %q", targetType)
	}

	// 2. Sort by Priority descending (99 down to 0)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Priority() > candidates[j].Priority()
	})

	// 3. Pick the highest priority driver that is available
	for _, candidate := range candidates {
		if candidate.IsAvailable(ctx) {
			return candidate, targetType, nil
		}
	}

	return nil, targetType, fmt.Errorf("no available downloader found for type %q (checked %d candidates)", targetType, len(candidates))
}

// AddURI routes AddURI to the highest priority available downloader
func (r *Registry) AddURI(ctx context.Context, uris []string, opts Options) (string, error) {
	if len(uris) == 0 {
		return "", fmt.Errorf("no uris provided")
	}

	target := uris[0]
	driver, targetType, err := r.SelectDownloader(ctx, target)
	if err != nil {
		return "", fmt.Errorf("failed to route uri %q (type %s): %w", target, targetType, err)
	}

	gid, err := driver.AddURI(ctx, uris, opts)
	if err != nil {
		return "", err
	}

	r.mu.Lock()
	r.gidToDriver[gid] = driver
	r.mu.Unlock()

	return gid, nil
}

// AddTorrent routes AddTorrent to the highest priority available downloader supporting BT
func (r *Registry) AddTorrent(ctx context.Context, base64Torrent string, opts Options) (string, error) {
	driver, _, err := r.SelectDownloader(ctx, "sample.torrent")
	if err != nil {
		return "", fmt.Errorf("failed to route torrent: %w", err)
	}

	gid, err := driver.AddTorrent(ctx, base64Torrent, opts)
	if err != nil {
		return "", err
	}

	r.mu.Lock()
	r.gidToDriver[gid] = driver
	r.mu.Unlock()

	return gid, nil
}

func (r *Registry) TellStatus(ctx context.Context, gid string) (*DownloadStatus, error) {
	r.mu.RLock()
	driver, ok := r.gidToDriver[gid]
	r.mu.RUnlock()

	if ok {
		return driver.TellStatus(ctx, gid)
	}

	// Try all drivers as fallback
	for _, d := range r.All() {
		if status, err := d.TellStatus(ctx, gid); err == nil && status != nil {
			return status, nil
		}
	}

	return nil, fmt.Errorf("task GID %s not found in any driver", gid)
}

func (r *Registry) ForceRemove(ctx context.Context, gid string) error {
	r.mu.RLock()
	driver, ok := r.gidToDriver[gid]
	r.mu.RUnlock()

	if ok {
		return driver.ForceRemove(ctx, gid)
	}

	for _, d := range r.All() {
		_ = d.ForceRemove(ctx, gid)
	}
	return nil
}

func (r *Registry) ResolveTargetPath(status *DownloadStatus) string {
	if status == nil {
		return ""
	}

	r.mu.RLock()
	driver, ok := r.gidToDriver[status.GID]
	r.mu.RUnlock()

	if ok {
		return driver.ResolveTargetPath(status)
	}

	for _, d := range r.All() {
		if path := d.ResolveTargetPath(status); path != "" {
			return path
		}
	}

	if len(status.Files) > 0 {
		return status.Files[0].Path
	}
	return ""
}

// DownloaderInfo provides diagnostic information about registered downloaders
type DownloaderInfo struct {
	Name        string       `json:"name"`
	Priority    int          `json:"priority"`
	TargetTypes []TargetType `json:"target_types"`
	Available   bool         `json:"available"`
}

func (r *Registry) GetDriversInfo(ctx context.Context) []DownloaderInfo {
	var infos []DownloaderInfo
	for _, d := range r.All() {
		infos = append(infos, DownloaderInfo{
			Name:        d.Name(),
			Priority:    d.Priority(),
			TargetTypes: d.TargetTypes(),
			Available:   d.IsAvailable(ctx),
		})
	}
	return infos
}

// Satisfy Downloader interface
func (r *Registry) Name() string {
	return "registry_router"
}

func (r *Registry) TargetTypes() []TargetType {
	return []TargetType{TargetHTTP, TargetBT, TargetMagnet, TargetVideo}
}

func (r *Registry) Priority() int {
	return 99
}

func (r *Registry) IsAvailable(ctx context.Context) bool {
	return true
}
