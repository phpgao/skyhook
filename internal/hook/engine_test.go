package hook_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/phpgao/skyhook/internal/executor"
	"github.com/phpgao/skyhook/internal/hook"
	"github.com/phpgao/skyhook/internal/model"
	"github.com/phpgao/skyhook/internal/notifier"
)

func TestHookEngine_OnCompleteCommandsAndInterpolation(t *testing.T) {
	mockExec := executor.NewMockExecutor()
	mockNotify := notifier.NewMockNotifier()
	engine := hook.NewStandardHookEngine(mockExec, mockNotify)

	hookCfg := &model.HookConfig{
		Notify: true,
		Commands: []model.HookCommand{
			{
				Name:            "upload-oss",
				Run:             "ossutil cp \"{path}\" oss://bucket/{name}",
				Timeout:         2 * time.Second,
				Retries:         1,
				ContinueOnError: true,
			},
		},
	}

	hCtx := hook.HookContext{
		TaskID:     "task-100",
		URL:        "https://example.com/data.zip",
		Path:       "/var/downloads/data.zip",
		Name:       "data.zip",
		Size:       1024,
		Downloader: "aria2_cli",
	}

	res, err := engine.Trigger(context.Background(), model.HookOnComplete, hookCfg, hCtx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Success {
		t.Errorf("expected hook to succeed")
	}

	// Verify command executed with interpolated variables
	if len(mockExec.CallHistory) != 1 {
		t.Fatalf("expected 1 command executed, got %d", len(mockExec.CallHistory))
	}
	expectedCmd := "ossutil cp \"/var/downloads/data.zip\" oss://bucket/data.zip"
	if mockExec.CallHistory[0] != expectedCmd {
		t.Errorf("expected command %q, got %q", expectedCmd, mockExec.CallHistory[0])
	}

	// Verify notification sent
	msgs := mockNotify.GetMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Title, "Hook: on_complete") {
		t.Errorf("expected on_complete in title, got: %s", msgs[0].Title)
	}
}

func TestHookEngine_OnStartNotification(t *testing.T) {
	mockExec := executor.NewMockExecutor()
	mockNotify := notifier.NewMockNotifier()
	engine := hook.NewStandardHookEngine(mockExec, mockNotify)

	hookCfg := &model.HookConfig{
		Notify: true,
	}

	hCtx := hook.HookContext{
		TaskID:     "task-200",
		URL:        "magnet:?xt=urn:btih:sample",
		Downloader: "aria2_rpc",
	}

	_, err := engine.Trigger(context.Background(), model.HookOnStart, hookCfg, hCtx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := mockNotify.GetMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 notification on start, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Title, "on_start") {
		t.Errorf("expected on_start notification, got: %s", msgs[0].Title)
	}
}
