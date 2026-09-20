package server

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/phpgao/skyhook/internal/config"
	"github.com/phpgao/skyhook/internal/downloader"
	"github.com/phpgao/skyhook/internal/hook"
	"github.com/phpgao/skyhook/internal/model"
	"github.com/phpgao/skyhook/internal/notifier"
	"github.com/phpgao/skyhook/internal/storage"
	"github.com/phpgao/skyhook/internal/watcher"
)

// Server is the HTTP server powered by Gin
type Server struct {
	appCtx      context.Context
	cfg         *config.Config
	downloader  downloader.Downloader
	watcher     *watcher.Watcher
	hookEngine  hook.Engine
	diskChecker storage.DiskChecker
	engine      *gin.Engine
}

// NewServer creates a new SkyHook Gin Server with a global application context
func NewServer(appCtx context.Context, cfg *config.Config, dl downloader.Downloader, w *watcher.Watcher, he hook.Engine) *Server {
	return NewServerWithDiskChecker(appCtx, cfg, dl, w, he, storage.NewOSDiskChecker())
}

// NewServerWithDiskChecker creates a new SkyHook Gin Server with custom DiskChecker
func NewServerWithDiskChecker(appCtx context.Context, cfg *config.Config, dl downloader.Downloader, w *watcher.Watcher, he hook.Engine, dc storage.DiskChecker) *Server {
	if appCtx == nil {
		appCtx = context.Background()
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(GlobalCtxMiddleware(appCtx))

	s := &Server{
		appCtx:      appCtx,
		cfg:         cfg,
		downloader:  dl,
		watcher:     w,
		hookEngine:  he,
		diskChecker: dc,
		engine:      r,
	}

	s.registerRoutes()
	return s
}

// SetDiskChecker allows overriding the disk checker
func (s *Server) SetDiskChecker(dc storage.DiskChecker) {
	s.diskChecker = dc
}

func (s *Server) registerRoutes() {
	// Public endpoints
	s.engine.GET("/health", s.handleHealth)
	s.engine.GET("/api/health", s.handleHealth)

	// Protected endpoints (Token Auth required)
	api := s.engine.Group("/api", AuthMiddleware(s.cfg.Server.AuthToken))
	{
		api.POST("/tasks", s.handleTasks)
		api.GET("/tasks", s.handleListTasks)
		api.GET("/tasks/:gid", s.handleGetTask)
	}
}

// Handler returns the HTTP handler (gin.Engine)
func (s *Server) Handler() http.Handler {
	return s.engine
}

// Engine returns the underlying gin.Engine
func (s *Server) Engine() *gin.Engine {
	return s.engine
}

func (s *Server) handleHealth(c *gin.Context) {
	// Using global context inside handler
	_ = GetGlobalCtx(c)

	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "skyhook",
	})
}

func (s *Server) handleTasks(c *gin.Context) {
	// Access the application-wide global context inside handler func
	globalCtx := GetGlobalCtx(c)

	var req model.TaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("invalid request payload: %v", err),
		})
		return
	}

	if len(req.URLs) == 0 && req.Torrent == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "either urls or torrent must be provided",
		})
		return
	}

	// 1. Resolve Action
	action, err := s.cfg.GetAction(req.ActionName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("failed to resolve action: %v", err),
		})
		return
	}

	// Custom notifier for this request
	var customNotify notifier.Notifier
	if req.NotifyURL != "" {
		customNotify = notifier.NewWebhookNotifier(req.NotifyURL, nil)
	}

	// 2. Resolve download directory and enforce disk reservation
	downloadDir := s.cfg.GetDownloadDir()
	_ = os.MkdirAll(downloadDir, 0755)

	minFreeBytes := s.cfg.GetMinFreeSpaceBytes()
	if s.diskChecker != nil && minFreeBytes > 0 {
		if err := s.diskChecker.CheckFreeSpace(downloadDir, minFreeBytes); err != nil {
			// Trigger on_create_failed hook
			if s.hookEngine != nil && action.Hooks.OnCreateFailed != nil {
				go func(errStr string) {
					targetURL := "[batch_request]"
					if len(req.URLs) > 0 {
						targetURL = req.URLs[0]
					}
					_, _ = s.hookEngine.Trigger(context.Background(), model.HookOnCreateFailed, action.Hooks.OnCreateFailed, hook.HookContext{
						URL:   targetURL,
						Error: errStr,
					}, customNotify)
				}(err.Error())
			}

			c.JSON(http.StatusInsufficientStorage, gin.H{
				"error": fmt.Sprintf("rejected: %v", err),
			})
			return
		}
	}

	// 3. Build download options (limits, trackers)
	dlLimit := s.cfg.Limits.DefaultDownloadLimit
	if req.DownloadLimit != "" {
		dlLimit = req.DownloadLimit
	}
	upLimit := s.cfg.Limits.DefaultUploadLimit
	if req.UploadLimit != "" {
		upLimit = req.UploadLimit
	}

	opts := downloader.Options{
		DownloadLimit: dlLimit,
		UploadLimit:   upLimit,
		Dir:           downloadDir,
		Trackers:      s.cfg.Aria2.Trackers,
	}

	var taskIDs []string

	// 4. Process URLs (HTTP / HTTPS / Magnets / Videos) using globalCtx
	for _, u := range req.URLs {
		gid, err := s.downloader.AddURI(globalCtx, []string{u}, opts)
		if err != nil {
			// Trigger on_create_failed hook
			if s.hookEngine != nil && action.Hooks.OnCreateFailed != nil {
				go func(failedURL, errMsg string) {
					_, _ = s.hookEngine.Trigger(context.Background(), model.HookOnCreateFailed, action.Hooks.OnCreateFailed, hook.HookContext{
						URL:   failedURL,
						Error: errMsg,
					}, customNotify)
				}(u, err.Error())
			}

			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("failed to submit uri %q: %v", u, err),
			})
			return
		}
		taskIDs = append(taskIDs, gid)
		s.watcher.Track(gid, u, *action, req.NotifyURL)
	}

	// 4. Process Torrent (Base64) using globalCtx
	if req.Torrent != "" {
		gid, err := s.downloader.AddTorrent(globalCtx, req.Torrent, opts)
		if err != nil {
			// Trigger on_create_failed hook
			if s.hookEngine != nil && action.Hooks.OnCreateFailed != nil {
				go func(errMsg string) {
					_, _ = s.hookEngine.Trigger(context.Background(), model.HookOnCreateFailed, action.Hooks.OnCreateFailed, hook.HookContext{
						URL:   "[torrent_payload]",
						Error: errMsg,
					}, customNotify)
				}(err.Error())
			}

			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("failed to submit torrent: %v", err),
			})
			return
		}
		taskIDs = append(taskIDs, gid)
		s.watcher.Track(gid, "torrent-task", *action, req.NotifyURL)
	}

	c.JSON(http.StatusAccepted, model.TaskResponse{
		TaskIDs: taskIDs,
		Status:  "queued",
		Message: fmt.Sprintf("successfully queued %d tasks with action %q", len(taskIDs), action.Name),
	})
}

func (s *Server) handleListTasks(c *gin.Context) {
	_ = GetGlobalCtx(c)

	tasks, err := s.watcher.ListTasks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to list tasks: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total": len(tasks),
		"tasks": tasks,
	})
}

func (s *Server) handleGetTask(c *gin.Context) {
	// Access global context inside handler func
	_ = GetGlobalCtx(c)

	gid := c.Param("gid")
	if gid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing task gid"})
		return
	}

	task, ok := s.watcher.GetTask(gid)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	c.JSON(http.StatusOK, task)
}
