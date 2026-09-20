package watcher

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/phpgao/skyhook/internal/config"
	"github.com/phpgao/skyhook/internal/downloader"
	"github.com/phpgao/skyhook/internal/hook"
	"github.com/phpgao/skyhook/internal/model"
	"github.com/phpgao/skyhook/internal/notifier"
	"github.com/phpgao/skyhook/internal/pipeline"
	"github.com/phpgao/skyhook/internal/storage"
)

// TaskRecord alias to model.TaskRecord for backward compatibility
type TaskRecord = model.TaskRecord

// Watcher manages background tracking of downloads, progress inspection, and lifecycle hooks
type Watcher struct {
	downloader     downloader.Downloader
	pipelineEngine pipeline.Engine
	hookEngine     hook.Engine
	globalNotifier notifier.Notifier
	cfg            *config.Config
	diskChecker    storage.DiskChecker
	taskStore      storage.TaskStore

	mu    sync.RWMutex
	tasks map[string]*TaskRecord

	pollInterval time.Duration
}

// NewWatcher creates a new download watcher with persistence store
func NewWatcher(
	dl downloader.Downloader,
	pe pipeline.Engine,
	he hook.Engine,
	notify notifier.Notifier,
	cfg *config.Config,
) *Watcher {
	var store storage.TaskStore
	if cfg != nil && cfg.GetStateFile() != "" {
		store, _ = storage.NewJSONFileTaskStore(cfg.GetStateFile())
	}
	if store == nil {
		store = storage.NewMemoryTaskStore()
	}
	return NewWatcherWithStore(dl, pe, he, notify, cfg, storage.NewOSDiskChecker(), store)
}

// NewWatcherWithDiskChecker creates a watcher with a custom disk checker (useful for testing)
func NewWatcherWithDiskChecker(
	dl downloader.Downloader,
	pe pipeline.Engine,
	he hook.Engine,
	notify notifier.Notifier,
	cfg *config.Config,
	dc storage.DiskChecker,
) *Watcher {
	return NewWatcherWithStore(dl, pe, he, notify, cfg, dc, storage.NewMemoryTaskStore())
}

// NewWatcherWithStore creates a watcher with custom DiskChecker and TaskStore
func NewWatcherWithStore(
	dl downloader.Downloader,
	pe pipeline.Engine,
	he hook.Engine,
	notify notifier.Notifier,
	cfg *config.Config,
	dc storage.DiskChecker,
	store storage.TaskStore,
) *Watcher {
	return &Watcher{
		downloader:     dl,
		pipelineEngine: pe,
		hookEngine:     he,
		globalNotifier: notify,
		cfg:            cfg,
		diskChecker:    dc,
		taskStore:      store,
		tasks:          make(map[string]*TaskRecord),
		pollInterval:   2 * time.Second,
	}
}

// SetTaskStore overrides the persistent task store
func (w *Watcher) SetTaskStore(store storage.TaskStore) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.taskStore = store
}

// SetDiskChecker overrides the disk checker
func (w *Watcher) SetDiskChecker(dc storage.DiskChecker) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.diskChecker = dc
}

// SetPollInterval overrides the polling interval (useful for testing)
func (w *Watcher) SetPollInterval(d time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pollInterval = d
}

// Track registers a download GID to be watched
func (w *Watcher) Track(gid string, targetURL string, action model.Action, notifyURL string) {
	record := &TaskRecord{
		GID:       gid,
		URL:       targetURL,
		Status:    model.StatusDownloading,
		Action:    action,
		NotifyURL: notifyURL,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	w.mu.Lock()
	w.tasks[gid] = record
	w.mu.Unlock()

	if w.taskStore != nil {
		_ = w.taskStore.Save(record)
	}

	// 1. Trigger on_start Hook immediately
	var customNotify notifier.Notifier
	if notifyURL != "" {
		customNotify = notifier.NewWebhookNotifier(notifyURL, nil)
	}

	if w.hookEngine != nil && action.Hooks.OnStart != nil {
		go func() {
			_, _ = w.hookEngine.Trigger(context.Background(), model.HookOnStart, action.Hooks.OnStart, hook.HookContext{
				TaskID:     gid,
				URL:        targetURL,
				Downloader: w.downloader.Name(),
			}, customNotify)
		}()
	}

	go w.watchLoop(gid, targetURL, action, notifyURL)
}

// GetTask returns current task record
func (w *Watcher) GetTask(gid string) (*TaskRecord, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	t, ok := w.tasks[gid]
	if !ok {
		return nil, false
	}
	cpy := *t
	return &cpy, true
}

// ListTasks returns all tasks from store or memory
func (w *Watcher) ListTasks() ([]*TaskRecord, error) {
	if w.taskStore != nil {
		return w.taskStore.ListAll()
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	var list []*TaskRecord
	for _, t := range w.tasks {
		cpy := *t
		list = append(list, &cpy)
	}
	return list, nil
}

func (w *Watcher) updateStatus(gid string, status model.TaskStatus, errMsg string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if t, ok := w.tasks[gid]; ok {
		t.Status = status
		t.ErrorMsg = errMsg
		t.UpdatedAt = time.Now()
	}
	if w.taskStore != nil {
		_ = w.taskStore.UpdateStatus(gid, status, errMsg)
	}
}

func (w *Watcher) watchLoop(gid string, targetURL string, action model.Action, notifyURL string) {
	timeout := w.cfg.Limits.DownloadTimeout
	if timeout <= 0 {
		timeout = 2 * time.Hour
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var customNotify notifier.Notifier
	if notifyURL != "" {
		customNotify = notifier.NewWebhookNotifier(notifyURL, nil)
	}

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	lastProgressReport := time.Now()

	for {
		select {
		case <-ctx.Done():
			// Download timed out!
			w.updateStatus(gid, model.StatusCanceled, "download timeout exceeded")
			_ = w.downloader.ForceRemove(context.Background(), gid)
			w.cleanupDownloadedFiles(gid)

			// Trigger on_error Hook
			if w.hookEngine != nil && action.Hooks.OnError != nil {
				_, _ = w.hookEngine.Trigger(context.Background(), model.HookOnError, action.Hooks.OnError, hook.HookContext{
					TaskID: gid,
					URL:    targetURL,
					Error:  fmt.Sprintf("下载超过最大超时时间 %s", timeout),
				}, customNotify)
			}
			return

		case <-ticker.C:
			status, err := w.downloader.TellStatus(ctx, gid)
			if err != nil {
				continue
			}

			// 1. Disk reservation and capacity check
			minFreeBytes := w.cfg.GetMinFreeSpaceBytes()
			if w.diskChecker != nil && minFreeBytes > 0 {
				downloadDir := w.cfg.GetDownloadDir()
				remainingBytes := uint64(0)
				if status.TotalLength > status.CompletedLength {
					remainingBytes = uint64(status.TotalLength - status.CompletedLength)
				}

				if err := w.diskChecker.CheckTaskSpace(downloadDir, remainingBytes, minFreeBytes); err != nil {
					errMsg := fmt.Sprintf("disk space reserve violated: %v", err)
					log.Printf("[Watcher 巡检] ⚠️ 任务 [%s] %s", gid, errMsg)
					w.updateStatus(gid, model.StatusFailed, errMsg)
					_ = w.downloader.ForceRemove(context.Background(), gid)
					w.cleanupDownloadedFiles(gid)

					if w.hookEngine != nil && action.Hooks.OnError != nil {
						go func(errStr string) {
							_, _ = w.hookEngine.Trigger(context.Background(), model.HookOnError, action.Hooks.OnError, hook.HookContext{
								TaskID: gid,
								URL:    targetURL,
								Error:  errStr,
							}, customNotify)
						}(errMsg)
					}
					return
				}
			}

			// Calculate progress percentage
			var progress float64
			if status.TotalLength > 0 {
				progress = (float64(status.CompletedLength) / float64(status.TotalLength)) * 100.0
			}

			// Output inspection log (任务巡检输出)
			speedStr := formatBytes(status.DownloadSpeed) + "/s"
			log.Printf("[Watcher 巡检] 任务 [%s] (%s) 状态: %s, 进度: %.1f%% (%s / %s), 速度: %s",
				gid, w.downloader.Name(), status.Status, progress,
				formatBytes(status.CompletedLength), formatBytes(status.TotalLength), speedStr)

			// If on_progress hook has notify enabled and 30s elapsed, send progress notification
			if action.Hooks.OnProgress != nil && action.Hooks.OnProgress.Notify && time.Since(lastProgressReport) >= 30*time.Second {
				lastProgressReport = time.Now()
				if w.hookEngine != nil {
					go func(prog float64, spd int64) {
						_, _ = w.hookEngine.Trigger(context.Background(), model.HookOnProgress, action.Hooks.OnProgress, hook.HookContext{
							TaskID:     gid,
							URL:        targetURL,
							Downloader: w.downloader.Name(),
							Progress:   prog,
							Speed:      spd,
							Size:       status.TotalLength,
						}, customNotify)
					}(progress, status.DownloadSpeed)
				}
			}

			switch status.Status {
			case "complete":
				w.processComplete(gid, targetURL, action, notifyURL, status)
				return

			case "error":
				w.updateStatus(gid, model.StatusFailed, status.ErrorMessage)
				w.cleanupDownloadedFiles(gid)

				// Trigger on_error Hook
				if w.hookEngine != nil && action.Hooks.OnError != nil {
					_, _ = w.hookEngine.Trigger(context.Background(), model.HookOnError, action.Hooks.OnError, hook.HookContext{
						TaskID: gid,
						URL:    targetURL,
						Error:  status.ErrorMessage,
					}, customNotify)
				}
				return

			case "removed":
				w.updateStatus(gid, model.StatusCanceled, "task removed")
				return
			}
		}
	}
}

func (w *Watcher) processComplete(gid string, targetURL string, action model.Action, notifyURL string, status *downloader.DownloadStatus) {
	var customNotify notifier.Notifier
	if notifyURL != "" {
		customNotify = notifier.NewWebhookNotifier(notifyURL, nil)
	}

	w.updateStatus(gid, model.StatusProcessing, "")
	targetPath := w.downloader.ResolveTargetPath(status)

	// Trigger on_complete Hook
	if w.hookEngine != nil && action.Hooks.OnComplete != nil {
		hookRes, err := w.hookEngine.Trigger(context.Background(), model.HookOnComplete, action.Hooks.OnComplete, hook.HookContext{
			TaskID:     gid,
			URL:        targetURL,
			Path:       targetPath,
			Name:       filepath.Base(targetPath),
			Size:       status.TotalLength,
			Downloader: w.downloader.Name(),
		}, customNotify)

		if err != nil || (hookRes != nil && !hookRes.Success) {
			w.updateStatus(gid, model.StatusFailed, "hook on_complete failed")
		} else {
			w.updateStatus(gid, model.StatusCompleted, "")
		}
	} else if w.pipelineEngine != nil {
		// Fallback to pipeline engine if hooks not configured
		res, err := w.pipelineEngine.Execute(context.Background(), targetPath, action, customNotify)
		if err != nil || (res != nil && !res.Success) {
			w.updateStatus(gid, model.StatusFailed, "pipeline execution failed")
		} else {
			w.updateStatus(gid, model.StatusCompleted, "")
		}
	} else {
		w.updateStatus(gid, model.StatusCompleted, "")
	}

	// Clean VPS disk if clean_at_end is set
	if action.CleanAtEnd && targetPath != "" {
		_ = os.RemoveAll(targetPath)
		log.Printf("[Cleaner] Cleaned target path: %s", targetPath)
	}
}

// RecoverAndResume checks for unfinished tasks in TaskStore across process restarts and resumes monitoring or 断点续传
func (w *Watcher) RecoverAndResume(ctx context.Context) (int, error) {
	if w.taskStore == nil {
		return 0, nil
	}
	unfinished, err := w.taskStore.ListUnfinished()
	if err != nil {
		return 0, fmt.Errorf("failed to load unfinished tasks from store: %w", err)
	}

	resumedCount := 0
	for _, record := range unfinished {
		log.Printf("[Watcher 恢复] 发现未完成任务 [%s] (%s), 正在探测状态...", record.GID, record.URL)
		w.mu.Lock()
		w.tasks[record.GID] = record
		w.mu.Unlock()

		status, err := w.downloader.TellStatus(ctx, record.GID)
		if err == nil && status != nil {
			if status.Status == "complete" {
				log.Printf("[Watcher 恢复] 任务 [%s] 在离线期间已下载完成，立即触发后续流水线...", record.GID)
				go w.processComplete(record.GID, record.URL, record.Action, record.NotifyURL, status)
				resumedCount++
				continue
			} else if status.Status == "active" || status.Status == "waiting" {
				log.Printf("[Watcher 恢复] 任务 [%s] 正在下载中，已无缝重新挂载巡检器与看门狗...", record.GID)
				go w.watchLoop(record.GID, record.URL, record.Action, record.NotifyURL)
				resumedCount++
				continue
			}
		}

		// Downloader process was restarted / lost: perform 断点续传 (Resume)
		log.Printf("[Watcher 断点续传] 任务 [%s] 下载中断，正在发起断点续传...", record.GID)
		opts := downloader.Options{
			Dir:           w.cfg.GetDownloadDir(),
			DownloadLimit: w.cfg.Limits.DefaultDownloadLimit,
			UploadLimit:   w.cfg.Limits.DefaultUploadLimit,
			Trackers:      w.cfg.Aria2.Trackers,
		}
		newGID, err := w.downloader.AddURI(ctx, []string{record.URL}, opts)
		if err != nil {
			log.Printf("[Watcher 断点续传] 任务 [%s] 续传失败: %v", record.GID, err)
			w.updateStatus(record.GID, model.StatusFailed, fmt.Sprintf("resume failed: %v", err))
			continue
		}

		log.Printf("[Watcher 断点续传] 任务已成功恢复续传，新任务ID [%s] (原任务: %s)", newGID, record.GID)
		if newGID != record.GID {
			_ = w.taskStore.Delete(record.GID)
			record.GID = newGID
		}
		w.Track(newGID, record.URL, record.Action, record.NotifyURL)
		resumedCount++
	}

	return resumedCount, nil
}

func (w *Watcher) cleanupDownloadedFiles(gid string) {
	status, err := w.downloader.TellStatus(context.Background(), gid)
	if err != nil || status == nil {
		return
	}
	for _, f := range status.Files {
		if f.Path != "" {
			_ = os.RemoveAll(f.Path)
			_ = os.Remove(f.Path + ".aria2")
		}
	}
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
