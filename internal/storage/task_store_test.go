package storage_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/phpgao/skyhook/internal/model"
	"github.com/phpgao/skyhook/internal/storage"
)

func TestJSONFileTaskStore_SaveGetList(t *testing.T) {
	tmpDir := t.TempDir()
	storeFile := filepath.Join(tmpDir, "tasks.json")

	store, err := storage.NewJSONFileTaskStore(storeFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	task1 := &model.TaskRecord{
		GID:       "task-101",
		URL:       "https://example.com/file1.zip",
		Status:    model.StatusDownloading,
		Action:    model.Action{Name: "archive"},
		CreatedAt: time.Now(),
	}
	task2 := &model.TaskRecord{
		GID:       "task-102",
		URL:       "magnet:?xt=urn:btih:hash2",
		Status:    model.StatusCompleted,
		Action:    model.Action{Name: "noop"},
		CreatedAt: time.Now().Add(time.Second),
	}

	// 1. Save tasks
	if err := store.Save(task1); err != nil {
		t.Fatalf("failed to save task1: %v", err)
	}
	if err := store.Save(task2); err != nil {
		t.Fatalf("failed to save task2: %v", err)
	}

	// 2. Get task
	got1, ok := store.Get("task-101")
	if !ok || got1.URL != task1.URL {
		t.Errorf("expected task-101 URL %s, got %v", task1.URL, got1)
	}

	// 3. UpdateStatus
	if err := store.UpdateStatus("task-101", model.StatusProcessing, ""); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	// 4. ListUnfinished (task-101 is processing -> unfinished; task-102 is completed -> finished)
	unfinished, err := store.ListUnfinished()
	if err != nil {
		t.Fatalf("failed to list unfinished: %v", err)
	}
	if len(unfinished) != 1 || unfinished[0].GID != "task-101" {
		t.Errorf("expected 1 unfinished task (task-101), got %d", len(unfinished))
	}

	// 5. Crash & Restart simulation: create a new store instance pointing to same file
	storeRestarted, err := storage.NewJSONFileTaskStore(storeFile)
	if err != nil {
		t.Fatalf("failed to reload store after restart: %v", err)
	}

	gotRestarted, ok := storeRestarted.Get("task-101")
	if !ok || gotRestarted.Status != model.StatusProcessing {
		t.Errorf("expected task-101 status 'processing' after restart, got %v", gotRestarted)
	}

	all, err := storeRestarted.ListAll()
	if err != nil {
		t.Fatalf("failed to list all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 tasks after restart, got %d", len(all))
	}
}
