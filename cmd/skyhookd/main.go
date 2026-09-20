package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/phpgao/skyhook/internal/config"
	"github.com/phpgao/skyhook/internal/downloader"
	"github.com/phpgao/skyhook/internal/executor"
	"github.com/phpgao/skyhook/internal/hook"
	"github.com/phpgao/skyhook/internal/notifier"
	"github.com/phpgao/skyhook/internal/pipeline"
	"github.com/phpgao/skyhook/internal/server"
	"github.com/phpgao/skyhook/internal/storage"
	"github.com/phpgao/skyhook/internal/watcher"
)

func main() {
	configPath := flag.String("c", "config.yaml", "Path to configuration file")
	flag.Parse()

	log.Printf("====================================================")
	log.Printf("      SkyHook (天钩) - Remote Relay & Automation     ")
	log.Printf("====================================================")

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration from %s: %v", *configPath, err)
	}

	// 1. Initialize Downloader Registry from config (priority-based routing)
	reg := downloader.BuildRegistryFromConfig(cfg)
	log.Printf("[Downloader Registry] Registered drivers:")
	for _, d := range reg.GetDriversInfo(context.Background()) {
		status := "✅ READY"
		if !d.Available {
			status = "❌ NOT FOUND / OFFLINE"
		}
		log.Printf("  • %-12s (Priority: %2d, Types: %v) -> %s", d.Name, d.Priority, d.TargetTypes, status)
	}

	// 2. Initialize Executor
	exec := executor.NewBashExecutor()

	// 3. Initialize Multi-Channel Notifiers (Telegram, Bark, WeCom, DingTalk, Feishu, Webhook)
	globalNotify := notifier.BuildFromConfig(cfg)

	// 4. Initialize Hook Engine & Pipeline Engine
	hookEngine := hook.NewStandardHookEngine(exec, globalNotify)
	engine := pipeline.NewStandardEngine(exec, globalNotify)

	// Application root context for global lifecycle
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	// 5. Initialize Storage, Disk Space Checker & Task Persistence Store
	diskChecker := storage.NewOSDiskChecker()
	dlDir := cfg.GetDownloadDir()
	_ = os.MkdirAll(dlDir, 0755)
	usage, err := diskChecker.GetDiskUsage(dlDir)
	if err == nil {
		log.Printf("[Storage] Directory: %s (Free: %s / Total: %s, Min Reserved: %s)",
			dlDir, storage.FormatBytes(usage.FreeBytes), storage.FormatBytes(usage.TotalBytes), cfg.Storage.MinFreeSpace)
	} else {
		log.Printf("[Storage] Directory: %s (Min Reserved: %s)", dlDir, cfg.Storage.MinFreeSpace)
	}

	stateFile := cfg.GetStateFile()
	taskStore, err := storage.NewJSONFileTaskStore(stateFile)
	if err != nil {
		log.Printf("WARNING: Failed to open state file %s: %v (using memory store)", stateFile, err)
	} else {
		log.Printf("[Persistence] Task state store initialized: %s", stateFile)
	}

	// 6. Initialize Download Watcher with Hook Engine, Disk Checker & Task Store
	taskWatcher := watcher.NewWatcherWithStore(reg, engine, hookEngine, globalNotify, cfg, diskChecker, taskStore)

	// 7. Recover & Resume unfinished downloads across process restarts
	resumed, err := taskWatcher.RecoverAndResume(rootCtx)
	if err != nil {
		log.Printf("[Recovery] Failed to recover unfinished tasks: %v", err)
	} else if resumed > 0 {
		log.Printf("[Recovery] Successfully recovered & resumed %d unfinished task(s)", resumed)
	}

	// 8. Initialize HTTP Server with Gin, Global Context & Disk Checker
	srv := server.NewServerWithDiskChecker(rootCtx, cfg, reg, taskWatcher, hookEngine, diskChecker)
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      srv.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Graceful shutdown handling
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("SkyHook daemon started on port %d", cfg.Server.Port)
		if cfg.Server.AuthToken != "" {
			log.Printf("Authentication enabled: token validation active")
		} else {
			log.Printf("WARNING: Authentication is disabled (no auth_token set)")
		}
		log.Printf("Default action: %q, Actions configured: %d", cfg.DefaultAction, len(cfg.Actions))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-stopChan
	log.Println("Shutting down SkyHook daemon...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	log.Println("SkyHook daemon stopped gracefully.")
}
