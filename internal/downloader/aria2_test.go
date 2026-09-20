package downloader_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/phpgao/skyhook/internal/downloader"
)

func TestAria2Client_AddURI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)

		if req["method"] != "aria2.addUri" {
			t.Errorf("expected method aria2.addUri, got %v", req["method"])
		}

		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      "skyhook",
			"result":  "12345",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := downloader.NewAria2Client(server.URL, "secret-token", nil)
	gid, err := client.AddURI(context.Background(), []string{"https://example.com/file.zip"}, downloader.Options{
		DownloadLimit: "5M",
		UploadLimit:   "1M",
	})
	if err != nil {
		t.Fatalf("unexpected error adding uri: %v", err)
	}

	if gid != "12345" {
		t.Errorf("expected GID 12345, got %s", gid)
	}
}

func TestAria2Client_TellStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      "skyhook",
			"result": map[string]interface{}{
				"gid":             "999",
				"status":          "complete",
				"totalLength":     "1048576",
				"completedLength": "1048576",
				"files": []map[string]interface{}{
					{
						"path":            "/tmp/downloads/sample.mp4",
						"length":          "1048576",
						"completedLength": "1048576",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := downloader.NewAria2Client(server.URL, "secret-token", nil)
	status, err := client.TellStatus(context.Background(), "999")
	if err != nil {
		t.Fatalf("failed to tell status: %v", err)
	}

	if status.Status != "complete" {
		t.Errorf("expected complete status, got %s", status.Status)
	}
	if status.TotalLength != 1048576 {
		t.Errorf("expected total length 1048576, got %d", status.TotalLength)
	}
	if len(status.Files) != 1 || status.Files[0].Path != "/tmp/downloads/sample.mp4" {
		t.Errorf("unexpected file list: %+v", status.Files)
	}
}

func TestAria2Client_ForceRemove(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      "skyhook",
			"result":  "OK",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := downloader.NewAria2Client(server.URL, "secret-token", nil)
	err := client.ForceRemove(context.Background(), "999")
	if err != nil {
		t.Fatalf("failed to force remove: %v", err)
	}
}
