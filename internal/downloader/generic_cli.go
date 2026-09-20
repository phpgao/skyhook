package downloader

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// GenericCLIDownloader executes any command-line tool configured in YAML
type GenericCLIDownloader struct {
	name         string
	bin          string // Executable path or binary name in PATH
	priority     int
	targetTypes  []TargetType
	argsTemplate []string
	downloadDir  string

	mu      sync.RWMutex
	tasks   map[string]*DownloadStatus
	nextGID int64
}

// NewGenericCLIDownloader creates a configurable CLI-driven downloader
func NewGenericCLIDownloader(
	name string,
	bin string,
	priority int,
	targetTypes []TargetType,
	argsTemplate []string,
	downloadDir string,
) *GenericCLIDownloader {
	if bin == "" {
		bin = name
	}
	if len(argsTemplate) == 0 {
		argsTemplate = getDefaultArgsForTool(name)
	}

	return &GenericCLIDownloader{
		name:         name,
		bin:          bin,
		priority:     priority,
		targetTypes:  targetTypes,
		argsTemplate: argsTemplate,
		downloadDir:  downloadDir,
		tasks:        make(map[string]*DownloadStatus),
		nextGID:      8000,
	}
}

func getDefaultArgsForTool(name string) []string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "aria2"):
		return []string{"-x", "16", "-s", "16", "-d", "{dir}", "{url}"}
	case strings.Contains(lower, "wget"):
		return []string{"-c", "-P", "{dir}", "{url}"}
	case strings.Contains(lower, "curl"):
		return []string{"-L", "-C", "-", "-o", "{path}", "{url}"}
	case strings.Contains(lower, "yt") || strings.Contains(lower, "dlp"):
		return []string{"-P", "{dir}", "--no-playlist", "-o", "%(title)s.%(ext)s", "{url}"}
	default:
		return []string{"{url}"}
	}
}

func (g *GenericCLIDownloader) Name() string {
	return g.name
}

func (g *GenericCLIDownloader) TargetTypes() []TargetType {
	return g.targetTypes
}

func (g *GenericCLIDownloader) Priority() int {
	return g.priority
}

func (g *GenericCLIDownloader) SetPriority(p int) {
	g.priority = p
}

// IsAvailable checks if bin exists either as an absolute path or in system PATH
func (g *GenericCLIDownloader) IsAvailable(ctx context.Context) bool {
	if strings.Contains(g.bin, "/") {
		fi, err := os.Stat(g.bin)
		return err == nil && !fi.IsDir()
	}
	_, err := exec.LookPath(g.bin)
	return err == nil
}

func (g *GenericCLIDownloader) AddURI(ctx context.Context, uris []string, opts Options) (string, error) {
	if len(uris) == 0 {
		return "", fmt.Errorf("no uris provided")
	}

	targetURI := uris[0]
	dir := g.downloadDir
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

	gidNum := atomic.AddInt64(&g.nextGID, 1)
	gid := fmt.Sprintf("%s-%d", g.name, gidNum)

	status := &DownloadStatus{
		GID:    gid,
		Status: "active",
		Dir:    dir,
		Files: []FileInfo{
			{Path: destPath},
		},
	}

	g.mu.Lock()
	g.tasks[gid] = status
	g.mu.Unlock()

	// Build arguments
	var cmdArgs []string
	for _, arg := range g.argsTemplate {
		a := strings.ReplaceAll(arg, "{dir}", dir)
		a = strings.ReplaceAll(a, "{url}", targetURI)
		a = strings.ReplaceAll(a, "{path}", destPath)
		a = strings.ReplaceAll(a, "{name}", fileName)
		if opts.DownloadLimit != "" && opts.DownloadLimit != "0" {
			a = strings.ReplaceAll(a, "{limit}", opts.DownloadLimit)
		}
		cmdArgs = append(cmdArgs, a)
	}

	// Resolve binary executable
	binPath := g.bin
	if resolved, err := exec.LookPath(g.bin); err == nil {
		binPath = resolved
	}

	go func() {
		cmd := exec.Command(binPath, cmdArgs...)
		out, err := cmd.CombinedOutput()

		g.mu.Lock()
		defer g.mu.Unlock()
		if t, ok := g.tasks[gid]; ok {
			if err != nil {
				t.Status = "error"
				t.ErrorMessage = fmt.Sprintf("%v: %s", err, string(out))
			} else {
				t.Status = "complete"
				if fi, err := os.Stat(destPath); err == nil {
					t.TotalLength = fi.Size()
					t.CompletedLength = fi.Size()
				} else {
					// For yt-dlp or multi-file downloads, find newest file in dir
					matches, _ := filepath.Glob(filepath.Join(dir, "*"))
					if len(matches) > 0 {
						newest := matches[len(matches)-1]
						if nfi, err := os.Stat(newest); err == nil {
							t.Files = []FileInfo{{Path: newest, Length: nfi.Size(), Completed: nfi.Size()}}
							t.TotalLength = nfi.Size()
							t.CompletedLength = nfi.Size()
						}
					}
				}
			}
		}
	}()

	return gid, nil
}

func (g *GenericCLIDownloader) AddTorrent(ctx context.Context, base64Torrent string, opts Options) (string, error) {
	return "", fmt.Errorf("CLI tool %s does not support direct base64 torrent submission (use aria2_rpc)", g.name)
}

func (g *GenericCLIDownloader) TellStatus(ctx context.Context, gid string) (*DownloadStatus, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	t, ok := g.tasks[gid]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", gid)
	}
	cpy := *t
	return &cpy, nil
}

func (g *GenericCLIDownloader) ForceRemove(ctx context.Context, gid string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if t, ok := g.tasks[gid]; ok {
		t.Status = "removed"
		if len(t.Files) > 0 && t.Files[0].Path != "" {
			_ = os.Remove(t.Files[0].Path)
		}
	}
	return nil
}

func (g *GenericCLIDownloader) ResolveTargetPath(status *DownloadStatus) string {
	if status == nil || len(status.Files) == 0 {
		return ""
	}
	return status.Files[0].Path
}
