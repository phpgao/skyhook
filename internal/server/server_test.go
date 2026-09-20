package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/phpgao/skyhook/internal/config"
	"github.com/phpgao/skyhook/internal/downloader"
	"github.com/phpgao/skyhook/internal/executor"
	"github.com/phpgao/skyhook/internal/hook"
	"github.com/phpgao/skyhook/internal/model"
	"github.com/phpgao/skyhook/internal/notifier"
	"github.com/phpgao/skyhook/internal/pipeline"
	"github.com/phpgao/skyhook/internal/server"
	"github.com/phpgao/skyhook/internal/storage"
	"github.com/phpgao/skyhook/internal/watcher"
)

func setupTestServer(token string) (*server.Server, *downloader.MockDownloader) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			AuthToken: token,
		},
		DefaultAction: "default-act",
		Actions: map[string]model.Action{
			"default-act": {
				Name: "Default Action",
				Steps: []model.Step{
					{Name: "step1", Command: "echo 1", Timeout: 5 * time.Second},
				},
			},
		},
	}

	mockDl := downloader.NewMockDownloader()
	mockExec := executor.NewMockExecutor()
	mockNotify := notifier.NewMockNotifier()
	engine := pipeline.NewStandardEngine(mockExec, mockNotify)
	he := hook.NewStandardHookEngine(mockExec, mockNotify)
	mockDisk := storage.NewMockDiskChecker(100*1024*1024*1024, 50*1024*1024*1024)
	mockStore := storage.NewMemoryTaskStore()
	w := watcher.NewWatcherWithStore(mockDl, engine, he, mockNotify, cfg, mockDisk, mockStore)

	appCtx := context.Background()
	srv := server.NewServerWithDiskChecker(appCtx, cfg, mockDl, w, he, mockDisk)
	return srv, mockDl
}

func TestServer_AuthTokenRequired(t *testing.T) {
	srv, _ := setupTestServer("my-secret-token")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 1. Health check is public -> 200 OK
	healthResp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("failed health request: %v", err)
	}
	defer healthResp.Body.Close()
	if healthResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for health, got %d", healthResp.StatusCode)
	}

	// 2. Request without token -> 401
	payload := `{"urls": ["https://example.com/file.iso"]}`
	resp, err := http.Post(ts.URL+"/api/tasks", "application/json", bytes.NewReader([]byte(payload)))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
	}

	// 3. Request with invalid token -> 401
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/tasks", bytes.NewReader([]byte(payload)))
	req.Header.Set("Authorization", "Bearer wrong-token")
	req.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for wrong token, got %d", resp2.StatusCode)
	}

	// 4. Request with valid Bearer token -> 202 Accepted
	req3, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/tasks", bytes.NewReader([]byte(payload)))
	req3.Header.Set("Authorization", "Bearer my-secret-token")
	req3.Header.Set("Content-Type", "application/json")
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp3.Body.Close()

	if resp3.StatusCode != http.StatusAccepted {
		t.Errorf("expected 202 Accepted for valid token, got %d", resp3.StatusCode)
	}

	// 5. Request with X-SkyHook-Token header -> 202 Accepted
	req4, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/tasks", bytes.NewReader([]byte(payload)))
	req4.Header.Set("X-SkyHook-Token", "my-secret-token")
	req4.Header.Set("Content-Type", "application/json")
	resp4, err := http.DefaultClient.Do(req4)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp4.Body.Close()

	if resp4.StatusCode != http.StatusAccepted {
		t.Errorf("expected 202 Accepted for X-SkyHook-Token, got %d", resp4.StatusCode)
	}
}

func TestServer_BatchSubmitAndQuery(t *testing.T) {
	srv, _ := setupTestServer("")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Submit batch of 3 URLs + 1 Torrent
	taskReq := model.TaskRequest{
		URLs: []string{
			"https://example.com/1.zip",
			"https://example.com/2.zip",
			"magnet:?xt=urn:btih:sample",
		},
		Torrent: "dGVzdC10b3JyZW50LWRhdGE=", // base64
	}

	data, _ := json.Marshal(taskReq)
	resp, err := http.Post(ts.URL+"/api/tasks", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to post tasks: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", resp.StatusCode)
	}

	var taskResp model.TaskResponse
	json.NewDecoder(resp.Body).Decode(&taskResp)

	// 3 URLs + 1 Torrent = 4 tasks
	if len(taskResp.TaskIDs) != 4 {
		t.Errorf("expected 4 task IDs, got %d", len(taskResp.TaskIDs))
	}

	// Query status of the first task
	firstGID := taskResp.TaskIDs[0]
	getResp, err := http.Get(ts.URL + "/api/tasks/" + firstGID)
	if err != nil {
		t.Fatalf("failed to query task: %v", err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", getResp.StatusCode)
	}

	// Query list of all tasks
	listResp, err := http.Get(ts.URL + "/api/tasks")
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 on list, got %d", listResp.StatusCode)
	}
	var listData map[string]interface{}
	json.NewDecoder(listResp.Body).Decode(&listData)
	if int(listData["total"].(float64)) != 4 {
		t.Errorf("expected 4 total tasks in list, got %v", listData["total"])
	}
}

func TestServer_DiskSpaceInsufficient_RejectsTask(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			AuthToken: "token-123",
		},
		Storage: config.StorageConfig{
			DownloadDir:  "/test/downloads",
			MinFreeSpace: "2GB",
		},
		DefaultAction: "default-act",
		Actions: map[string]model.Action{
			"default-act": {
				Name: "Default Action",
				Hooks: model.Hooks{
					OnCreateFailed: &model.HookConfig{
						Notify: true,
					},
				},
			},
		},
	}

	mockDl := downloader.NewMockDownloader()
	mockExec := executor.NewMockExecutor()
	mockNotify := notifier.NewMockNotifier()
	engine := pipeline.NewStandardEngine(mockExec, mockNotify)
	he := hook.NewStandardHookEngine(mockExec, mockNotify)

	// Free space 500MB is strictly less than MinFreeSpace 2GB
	mockDisk := storage.NewMockDiskChecker(100*1024*1024*1024, 500*1024*1024)
	w := watcher.NewWatcherWithDiskChecker(mockDl, engine, he, mockNotify, cfg, mockDisk)
	srv := server.NewServerWithDiskChecker(context.Background(), cfg, mockDl, w, he, mockDisk)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	taskReq := model.TaskRequest{
		URLs: []string{"https://example.com/hugefile.iso"},
	}
	data, _ := json.Marshal(taskReq)

	req, _ := http.NewRequest("POST", ts.URL+"/api/tasks", bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer token-123")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to post tasks: %v", err)
	}
	defer resp.Body.Close()

	// Must be rejected with HTTP 507 Insufficient Storage
	if resp.StatusCode != http.StatusInsufficientStorage {
		t.Errorf("expected status 507, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if !strings.Contains(body["error"], "insufficient disk space") {
		t.Errorf("expected error to mention 'insufficient disk space', got: %v", body["error"])
	}
}

