package notifier_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/phpgao/skyhook/internal/config"
	"github.com/phpgao/skyhook/internal/notifier"
)

func TestChannels_DingTalk_WeCom_Feishu(t *testing.T) {
	var receivedPayload map[string]interface{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	msg := notifier.Message{
		Title:     "任务开始",
		Body:      "开始下载 Ubuntu ISO",
		Level:     notifier.LevelInfo,
		Timestamp: time.Now(),
	}

	// 1. DingTalk
	ding := notifier.NewDingTalkNotifier(ts.URL, nil)
	if err := ding.Notify(context.Background(), msg); err != nil {
		t.Fatalf("DingTalk notification failed: %v", err)
	}
	if receivedPayload["msgtype"] != "markdown" {
		t.Errorf("expected dingtalk msgtype 'markdown', got %v", receivedPayload["msgtype"])
	}

	// 2. WeCom
	wecom := notifier.NewWeComNotifier(ts.URL, nil)
	if err := wecom.Notify(context.Background(), msg); err != nil {
		t.Fatalf("WeCom notification failed: %v", err)
	}
	if receivedPayload["msgtype"] != "markdown" {
		t.Errorf("expected wecom msgtype 'markdown', got %v", receivedPayload["msgtype"])
	}

	// 3. Feishu
	feishu := notifier.NewFeishuNotifier(ts.URL, nil)
	if err := feishu.Notify(context.Background(), msg); err != nil {
		t.Fatalf("Feishu notification failed: %v", err)
	}
	if receivedPayload["msg_type"] != "text" {
		t.Errorf("expected feishu msg_type 'text', got %v", receivedPayload["msg_type"])
	}
}

func TestBuildFromConfig(t *testing.T) {
	var receivedCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := &config.Config{
		Notify: config.NotifyConfig{
			Channels: []config.ChannelConfig{
				{Name: "ding", Type: "dingtalk", URL: ts.URL},
				{Name: "wecom", Type: "wecom", URL: ts.URL},
				{Name: "feishu", Type: "feishu", URL: ts.URL},
				{Name: "hook", Type: "webhook", URL: ts.URL},
			},
		},
	}

	multi := notifier.BuildFromConfig(cfg)
	msg := notifier.Message{
		Title:     "巡检提醒",
		Body:      "下载完成 100%",
		Level:     notifier.LevelSuccess,
		Timestamp: time.Now(),
	}

	if err := multi.Notify(context.Background(), msg); err != nil {
		t.Fatalf("multi notification failed: %v", err)
	}

	// 4 channels should all have received the request
	if receivedCount != 4 {
		t.Errorf("expected 4 channel notifications, got %d", receivedCount)
	}
}
