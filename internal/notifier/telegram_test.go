package notifier_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/phpgao/skyhook/internal/notifier"
)

func TestTelegramNotifier_Success(t *testing.T) {
	var receivedPayload map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	// Replace Telegram API base via custom HTTP client / transport to test against local httptest
	tg := notifier.NewTelegramNotifier("fake-token", "12345678", &http.Client{
		Transport: &testRoundTripper{targetURL: server.URL},
		Timeout:   5 * time.Second,
	})

	err := tg.Notify(context.Background(), notifier.Message{
		Title: "任务开始",
		Body:  "正在下载: https://example.com/test.iso",
		Level: notifier.LevelInfo,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedPayload["chat_id"] != "12345678" {
		t.Errorf("expected chat_id 12345678, got %v", receivedPayload["chat_id"])
	}

	text := receivedPayload["text"].(string)
	if !strings.Contains(text, "任务开始") || !strings.Contains(text, "test.iso") {
		t.Errorf("unexpected text in telegram message: %s", text)
	}
}

type testRoundTripper struct {
	targetURL string
}

func (t *testRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	newReq, _ := http.NewRequestWithContext(req.Context(), req.Method, t.targetURL, req.Body)
	newReq.Header = req.Header
	return http.DefaultClient.Do(newReq)
}
