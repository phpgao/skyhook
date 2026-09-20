package watcher_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/phpgao/skyhook/internal/config"
	"github.com/phpgao/skyhook/internal/downloader"
	"github.com/phpgao/skyhook/internal/executor"
	"github.com/phpgao/skyhook/internal/hook"
	"github.com/phpgao/skyhook/internal/model"
	"github.com/phpgao/skyhook/internal/notifier"
	"github.com/phpgao/skyhook/internal/pipeline"
	"github.com/phpgao/skyhook/internal/storage"
	"github.com/phpgao/skyhook/internal/watcher"
)

func TestWatcher_SuccessfulDownloadTriggersPipeline(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "downloaded.bin")
	_ = os.WriteFile(targetFile, []byte("payload"), 0644)

	mockDl := downloader.NewMockDownloader()
	mockExec := executor.NewMockExecutor()
	mockNotify := notifier.NewMockNotifier()

	cfg := &config.Config{
		Limits: config.LimitsConfig{
			DownloadTimeout: 5 * time.Second,
		},
	}

	engine := pipeline.NewStandardEngine(mockExec, mockNotify)
	he := hook.NewStandardHookEngine(mockExec, mockNotify)
	w := watcher.NewWatcher(mockDl, engine, he, mockNotify, cfg)
	w.SetPollInterval(15 * time.Millisecond)

	gid := "2001"
	mockDl.Tasks[gid] = &downloader.DownloadStatus{
		GID:    gid,
		Status: "active",
	}

	action := model.Action{
		Name: "test-pipeline",
		Steps: []model.Step{
			{Name: "step1", Command: "echo processed", Timeout: 1 * time.Second},
		},
	}

	w.Track(gid, "https://example.com/downloaded.bin", action, "")

	// Simulate download completing after 50ms
	time.Sleep(50 * time.Millisecond)
	mockDl.SetTaskStatus(gid, "complete", []downloader.FileInfo{
		{Path: targetFile, Length: 7, Completed: 7},
	}, targetFile)

	// Wait for watcher loop to detect and process
	time.Sleep(200 * time.Millisecond)

	task, ok := w.GetTask(gid)
	if !ok {
		t.Fatalf("task not found in watcher")
	}

	// Should have executed pipeline
	if task.Status != model.StatusCompleted {
		t.Errorf("expected task status %s, got %s", model.StatusCompleted, task.Status)
	}

	// Check executor history
	if len(mockExec.CallHistory) != 1 {
		t.Errorf("expected 1 execution in pipeline, got %d", len(mockExec.CallHistory))
	}
}

func TestWatcher_DownloadTimeout(t *testing.T) {
	mockDl := downloader.NewMockDownloader()
	mockExec := executor.NewMockExecutor()
	mockNotify := notifier.NewMockNotifier()

	// 50ms download timeout
	cfg := &config.Config{
		Limits: config.LimitsConfig{
			DownloadTimeout: 50 * time.Millisecond,
		},
	}

	engine := pipeline.NewStandardEngine(mockExec, mockNotify)
	he := hook.NewStandardHookEngine(mockExec, mockNotify)
	w := watcher.NewWatcher(mockDl, engine, he, mockNotify, cfg)
	w.SetPollInterval(15 * time.Millisecond)

	gid := "2002"
	mockDl.Tasks[gid] = &downloader.DownloadStatus{
		GID:    gid,
		Status: "active",
	}

	w.Track(gid, "https://example.com/deadlink.zip", model.Action{Name: "noop"}, "")

	// Wait for timeout to trigger
	time.Sleep(150 * time.Millisecond)

	task, ok := w.GetTask(gid)
	if !ok {
		t.Fatalf("task not found in watcher")
	}

	if task.Status != model.StatusCanceled {
		t.Errorf("expected task status %s on timeout, got %s", model.StatusCanceled, task.Status)
	}

	// Downloader ForceRemove should have been called
	if !mockDl.IsForceRemoved(gid) {
		t.Errorf("expected downloader.ForceRemove to be called on timeout")
	}
}

func TestWatcher_DiskSpaceInsufficient_AbortsDownload(t *testing.T) {
	mockDl := downloader.NewMockDownloader()
	mockExec := executor.NewMockExecutor()
	mockNotify := notifier.NewMockNotifier()

	cfg := &config.Config{
		Storage: config.StorageConfig{
			DownloadDir:  "/downloads",
			MinFreeSpace: "2GB",
		},
		Limits: config.LimitsConfig{
			DownloadTimeout: 5 * time.Second,
		},
	}

	engine := pipeline.NewStandardEngine(mockExec, mockNotify)
	he := hook.NewStandardHookEngine(mockExec, mockNotify)

	// Available disk: 5GB, MinFreeSpace: 2GB (Safe to allocate up to 3GB)
	mockDisk := storage.NewMockDiskChecker(100*1024*1024*1024, 5*1024*1024*1024)
	w := watcher.NewWatcherWithDiskChecker(mockDl, engine, he, mockNotify, cfg, mockDisk)
	w.SetPollInterval(15 * time.Millisecond)

	gid := "2003"
	// Task requires 10GB (> 3GB allowable)
	mockDl.Tasks[gid] = &downloader.DownloadStatus{
		GID:             gid,
		Status:          "active",
		TotalLength:     10 * 1024 * 1024 * 1024,
		CompletedLength: 0,
	}

	w.Track(gid, "magnet:?xt=urn:btih:giant-file", model.Action{Name: "noop"}, "")

	// Wait for watcher loop to inspect and detect disk violation
	time.Sleep(100 * time.Millisecond)

	task, ok := w.GetTask(gid)
	if !ok {
		t.Fatalf("task not found in watcher")
	}

	if task.Status != model.StatusFailed {
		t.Errorf("expected task status %s on disk insufficiency, got %s", model.StatusFailed, task.Status)
	}

	if !mockDl.IsForceRemoved(gid) {
		t.Errorf("expected downloader.ForceRemove to be called on disk space violation")
	}
}

func TestWatcher_RecoverAndResume(t *testing.T) {
	tmpDir := t.TempDir()
	storeFile := filepath.Join(tmpDir, "tasks.json")
	store, err := storage.NewJSONFileTaskStore(storeFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	mockDl := downloader.NewMockDownloader()
	mockExec := executor.NewMockExecutor()
	mockNotify := notifier.NewMockNotifier()

	cfg := &config.Config{
		Storage: config.StorageConfig{
			DownloadDir: tmpDir,
			StateFile:   storeFile,
		},
	}

	// Pre-seed store with an unfinished task that completed while daemon was offline
	targetFile := filepath.Join(tmpDir, "offline-done.iso")
	_ = os.WriteFile(targetFile, []byte("data"), 0644)

	task1 := &model.TaskRecord{
		GID:       "task-offline",
		URL:       "https://example.com/offline-done.iso",
		Status:    model.StatusDownloading,
		Action:    model.Action{Name: "noop"},
		CreatedAt: time.Now(),
	}
	_ = store.Save(task1)

	// Simulate downloader reporting it as already complete
	mockDl.Tasks["task-offline"] = &downloader.DownloadStatus{
		GID:             "task-offline",
		Status:          "complete",
		TotalLength:     4,
		CompletedLength: 4,
		Files: []downloader.FileInfo{
			{Path: targetFile, Length: 4, Completed: 4},
		},
	}

	engine := pipeline.NewStandardEngine(mockExec, mockNotify)
	he := hook.NewStandardHookEngine(mockExec, mockNotify)
	w := watcher.NewWatcherWithStore(mockDl, engine, he, mockNotify, cfg, storage.NewMockDiskChecker(1000, 1000), store)

	// Run RecoverAndResume
	resumed, err := w.RecoverAndResume(context.Background())
	if err != nil {
		t.Fatalf("RecoverAndResume failed: %v", err)
	}
	if resumed != 1 {
		t.Errorf("expected 1 resumed task, got %d", resumed)
	}

	time.Sleep(50 * time.Millisecond)

	// Task should now be marked as completed in store and watcher
	t1, ok := w.GetTask("task-offline")
	if !ok {
		t.Fatalf("task-offline not found in watcher")
	}
	if t1.Status != model.StatusCompleted {
		t.Errorf("expected task-offline status 'completed', got %s", t1.Status)
	}
}


