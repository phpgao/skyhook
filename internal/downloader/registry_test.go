package downloader_test

import (
	"context"
	"testing"

	"github.com/phpgao/skyhook/internal/downloader"
)

func TestDetectTargetType(t *testing.T) {
	cases := []struct {
		input    string
		expected downloader.TargetType
	}{
		{"https://example.com/file.zip", downloader.TargetHTTP},
		{"http://site.org/archive.tar.gz", downloader.TargetHTTP},
		{"magnet:?xt=urn:btih:d4399e", downloader.TargetMagnet},
		{"/path/to/ubuntu-22.04.torrent", downloader.TargetBT},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", downloader.TargetVideo},
		{"https://youtu.be/dQw4w9WgXcQ", downloader.TargetVideo},
		{"https://www.bilibili.com/video/BV1xx411c7mD", downloader.TargetVideo},
	}

	for _, c := range cases {
		got := downloader.DetectTargetType(c.input, nil)
		if got != c.expected {
			t.Errorf("DetectTargetType(%q) = %s, expected %s", c.input, got, c.expected)
		}
	}
}

type dummyDriver struct {
	name      string
	types     []downloader.TargetType
	priority  int
	available bool
}

func (d *dummyDriver) Name() string                                    { return d.name }
func (d *dummyDriver) TargetTypes() []downloader.TargetType            { return d.types }
func (d *dummyDriver) Priority() int                                   { return d.priority }
func (d *dummyDriver) IsAvailable(ctx context.Context) bool            { return d.available }
func (d *dummyDriver) AddURI(ctx context.Context, u []string, o downloader.Options) (string, error) {
	return "gid-" + d.name, nil
}
func (d *dummyDriver) AddTorrent(ctx context.Context, b string, o downloader.Options) (string, error) {
	return "gid-torrent-" + d.name, nil
}
func (d *dummyDriver) TellStatus(ctx context.Context, g string) (*downloader.DownloadStatus, error) {
	return &downloader.DownloadStatus{GID: g, Status: "complete"}, nil
}
func (d *dummyDriver) ForceRemove(ctx context.Context, g string) error { return nil }
func (d *dummyDriver) ResolveTargetPath(s *downloader.DownloadStatus) string {
	return "/tmp/" + d.name
}

func TestRegistry_PriorityRouting(t *testing.T) {
	reg := downloader.NewRegistry(nil)

	// Register drivers with the specified priorities:
	// wget: 19
	// curl: 10
	// aria2c: 88
	// native: 1
	// yt-dlp: 10 (for video)
	// aria2_rpc: 88 (for bt, magnet)

	aria2CLI := &dummyDriver{name: "aria2c", priority: 88, types: []downloader.TargetType{downloader.TargetHTTP}, available: true}
	wget := &dummyDriver{name: "wget", priority: 19, types: []downloader.TargetType{downloader.TargetHTTP}, available: true}
	curl := &dummyDriver{name: "curl", priority: 10, types: []downloader.TargetType{downloader.TargetHTTP}, available: true}
	native := &dummyDriver{name: "native", priority: 1, types: []downloader.TargetType{downloader.TargetHTTP}, available: true}
	ytdlp := &dummyDriver{name: "yt-dlp", priority: 10, types: []downloader.TargetType{downloader.TargetVideo}, available: true}
	aria2RPC := &dummyDriver{name: "aria2_rpc", priority: 88, types: []downloader.TargetType{downloader.TargetBT, downloader.TargetMagnet}, available: true}

	reg.Register(native)
	reg.Register(curl)
	reg.Register(wget)
	reg.Register(aria2CLI)
	reg.Register(ytdlp)
	reg.Register(aria2RPC)

	ctx := context.Background()

	// 1. HTTP target with aria2c available -> should select aria2c (priority 88 > 19 > 10 > 1)
	d1, targetType, err := reg.SelectDownloader(ctx, "https://example.com/file.iso")
	if err != nil {
		t.Fatalf("unexpected select error: %v", err)
	}
	if targetType != downloader.TargetHTTP || d1.Name() != "aria2c" {
		t.Errorf("expected aria2c (priority 88), got %s", d1.Name())
	}

	// 2. If aria2c is not available -> should select wget (priority 19 > 10 > 1)
	aria2CLI.available = false
	d2, _, err := reg.SelectDownloader(ctx, "https://example.com/file.iso")
	if err != nil {
		t.Fatalf("unexpected select error: %v", err)
	}
	if d2.Name() != "wget" {
		t.Errorf("expected wget (priority 19), got %s", d2.Name())
	}

	// 3. If wget is also not available -> should select curl (priority 10 > 1)
	wget.available = false
	d3, _, err := reg.SelectDownloader(ctx, "https://example.com/file.iso")
	if err != nil {
		t.Fatalf("unexpected select error: %v", err)
	}
	if d3.Name() != "curl" {
		t.Errorf("expected curl (priority 10), got %s", d3.Name())
	}

	// 4. If curl is also not available -> should fallback to native (priority 1)
	curl.available = false
	d4, _, err := reg.SelectDownloader(ctx, "https://example.com/file.iso")
	if err != nil {
		t.Fatalf("unexpected select error: %v", err)
	}
	if d4.Name() != "native" {
		t.Errorf("expected native (priority 1), got %s", d4.Name())
	}

	// 5. Magnet link -> selects aria2_rpc (priority 88)
	d5, tt5, err := reg.SelectDownloader(ctx, "magnet:?xt=urn:btih:xyz")
	if err != nil {
		t.Fatalf("unexpected select error: %v", err)
	}
	if tt5 != downloader.TargetMagnet || d5.Name() != "aria2_rpc" {
		t.Errorf("expected aria2_rpc, got %s (type %s)", d5.Name(), tt5)
	}

	// 6. YouTube video -> selects yt-dlp
	d6, tt6, err := reg.SelectDownloader(ctx, "https://www.youtube.com/watch?v=123")
	if err != nil {
		t.Fatalf("unexpected select error: %v", err)
	}
	if tt6 != downloader.TargetVideo || d6.Name() != "yt-dlp" {
		t.Errorf("expected yt-dlp, got %s (type %s)", d6.Name(), tt6)
	}
}
