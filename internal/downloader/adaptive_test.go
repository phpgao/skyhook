package downloader_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phpgao/skyhook/internal/downloader"
)

func TestNativeHTTPDownloader_DownloadSuccess(t *testing.T) {
	testData := "hello from mock http server for skyhook"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "39")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(testData))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	native := downloader.NewNativeHTTPDownloader(tmpDir, nil)

	gid, err := native.AddURI(context.Background(), []string{server.URL + "/archive.zip"}, downloader.Options{
		Dir: tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to add uri: %v", err)
	}

	if !strings.HasPrefix(gid, "native-") {
		t.Errorf("expected native- gid, got %s", gid)
	}

	// Wait for background download to finish
	time.Sleep(100 * time.Millisecond)

	status, err := native.TellStatus(context.Background(), gid)
	if err != nil {
		t.Fatalf("failed to tell status: %v", err)
	}

	if status.Status != "complete" {
		t.Errorf("expected complete status, got %s", status.Status)
	}

	targetPath := native.ResolveTargetPath(status)
	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}

	if string(content) != testData {
		t.Errorf("expected %q, got %q", testData, string(content))
	}
}

func TestAdaptiveDownloader_FallbackWhenAria2Offline(t *testing.T) {
	// Point aria2 to dead port
	aria2Dead := downloader.NewAria2Client("http://127.0.0.1:59999/jsonrpc", "", nil)

	tmpDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fallback data"))
	}))
	defer server.Close()

	native := downloader.NewNativeHTTPDownloader(tmpDir, nil)
	adaptive := downloader.NewAdaptiveDownloader(aria2Dead, native)

	if adaptive.IsAria2Online() {
		t.Errorf("expected Aria2 to be detected as offline")
	}

	// 1. Magnet should gracefully fail with clear warning
	_, err := adaptive.AddURI(context.Background(), []string{"magnet:?xt=urn:btih:sample"}, downloader.Options{})
	if err == nil || !strings.Contains(err.Error(), "Aria2 is offline") {
		t.Errorf("expected magnet to fail with Aria2 offline error, got %v", err)
	}

	// 2. .torrent should fail gracefully
	_, err = adaptive.AddTorrent(context.Background(), "base64", downloader.Options{})
	if err == nil || !strings.Contains(err.Error(), "Aria2 is offline") {
		t.Errorf("expected torrent to fail with Aria2 offline error, got %v", err)
	}

	// 3. HTTP/HTTPS should seamlessly fallback to native downloader!
	gid, err := adaptive.AddURI(context.Background(), []string{server.URL + "/file.tar.gz"}, downloader.Options{
		Dir: tmpDir,
	})
	if err != nil {
		t.Fatalf("expected HTTP to fallback to native downloader, got error: %v", err)
	}

	if !strings.HasPrefix(gid, "native-") {
		t.Errorf("expected native- gid on fallback, got %s", gid)
	}

	time.Sleep(100 * time.Millisecond)

	status, err := adaptive.TellStatus(context.Background(), gid)
	if err != nil {
		t.Fatalf("failed to query status: %v", err)
	}

	if status.Status != "complete" {
		t.Errorf("expected complete status, got %s", status.Status)
	}

	expectedFile := filepath.Join(tmpDir, "file.tar.gz")
	if _, err := os.Stat(expectedFile); err != nil {
		t.Errorf("expected file %s to exist: %v", expectedFile, err)
	}
}
