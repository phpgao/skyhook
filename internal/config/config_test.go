package config_test

import (
	"testing"
	"time"

	"github.com/phpgao/skyhook/internal/config"
)

func TestConfigLoadWithDurations(t *testing.T) {
	yamlContent := `
server:
  port: 9090
  auth_token: "secret-token-123"

aria2:
  rpc_url: "http://10.0.0.1:6800/jsonrpc"
  rpc_secret: "aria2-pwd"
  download_dir: "/var/downloads"
  trackers: "udp://tracker.opentrackr.org:1337/announce"

limits:
  default_download_limit: "10M"
  default_upload_limit: "2M"
  download_timeout: 3h30m

default_action: "backup"

actions:
  backup:
    name: "Multi-destination Backup"
    clean_at_end: true
    steps:
      - name: "Upload to Quark"
        command: "quark-cli upload \"{path}\" /remote/"
        timeout: 45m
        retries: 3
        retry_interval: 10s
        continue_on_error: true
      - name: "Upload to OSS"
        command: "ossutil cp \"{path}\" oss://my-bucket/"
        timeout: 1h
        retries: 2
        retry_interval: 5s
        continue_on_error: false
`

	cfg, err := config.LoadFromBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	// Verify server config & token
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.AuthToken != "secret-token-123" {
		t.Errorf("expected token 'secret-token-123', got %s", cfg.Server.AuthToken)
	}

	// Verify duration parsing
	expectedTimeout := 3*time.Hour + 30*time.Minute
	if cfg.Limits.DownloadTimeout != expectedTimeout {
		t.Errorf("expected download_timeout %v, got %v", expectedTimeout, cfg.Limits.DownloadTimeout)
	}

	// Verify default action
	action, err := cfg.GetAction("")
	if err != nil {
		t.Fatalf("failed to get default action: %v", err)
	}
	if action.Name != "Multi-destination Backup" {
		t.Errorf("expected action name 'Multi-destination Backup', got %s", action.Name)
	}
	if !action.CleanAtEnd {
		t.Errorf("expected CleanAtEnd to be true")
	}
	if len(action.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(action.Steps))
	}

	// Verify step 1 duration and retries
	step1 := action.Steps[0]
	if step1.Timeout != 45*time.Minute {
		t.Errorf("expected step 1 timeout 45m, got %v", step1.Timeout)
	}
	if step1.Retries != 3 {
		t.Errorf("expected 3 retries, got %d", step1.Retries)
	}
	if step1.RetryInterval != 10*time.Second {
		t.Errorf("expected retry_interval 10s, got %v", step1.RetryInterval)
	}
	if !step1.ContinueOnError {
		t.Errorf("expected continue_on_error to be true for step 1")
	}

	// Verify step 2
	step2 := action.Steps[1]
	if step2.Timeout != 1*time.Hour {
		t.Errorf("expected step 2 timeout 1h, got %v", step2.Timeout)
	}
	if step2.ContinueOnError {
		t.Errorf("expected continue_on_error to be false for step 2")
	}
}

func TestConfigDefaultsAndValidation(t *testing.T) {
	// Minimal configuration
	yamlContent := `
server:
  auth_token: "token"
`
	cfg, err := config.LoadFromBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("failed to load minimal config: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Limits.DownloadTimeout != 2*time.Hour {
		t.Errorf("expected default download timeout 2h, got %v", cfg.Limits.DownloadTimeout)
	}
	if cfg.Limits.DefaultUploadLimit != "1M" {
		t.Errorf("expected default upload limit 1M, got %s", cfg.Limits.DefaultUploadLimit)
	}
	if cfg.GetDownloadDir() != "/tmp/skyhook_downloads" {
		t.Errorf("expected default download dir '/tmp/skyhook_downloads', got %s", cfg.GetDownloadDir())
	}
	if cfg.GetMinFreeSpaceBytes() != 1024*1024*1024 {
		t.Errorf("expected default min free space 1GB (1073741824), got %d", cfg.GetMinFreeSpaceBytes())
	}
}

func TestConfigStorageExplicit(t *testing.T) {
	yamlContent := `
storage:
  download_dir: "/data/custom_downloads"
  min_free_space: "5GB"
`
	cfg, err := config.LoadFromBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GetDownloadDir() != "/data/custom_downloads" {
		t.Errorf("expected '/data/custom_downloads', got %s", cfg.GetDownloadDir())
	}
	expected5G := uint64(5 * 1024 * 1024 * 1024)
	if cfg.GetMinFreeSpaceBytes() != expected5G {
		t.Errorf("expected 5GB (%d), got %d", expected5G, cfg.GetMinFreeSpaceBytes())
	}
}

