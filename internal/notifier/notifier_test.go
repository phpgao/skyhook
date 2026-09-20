package notifier_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/phpgao/skyhook/internal/notifier"
)

func TestWebhookNotifier_Success(t *testing.T) {
	var received map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := notifier.NewWebhookNotifier(server.URL, nil)
	err := n.Notify(context.Background(), notifier.Message{
		Title:     "Download Done",
		Body:      "File: ubuntu.iso",
		Level:     notifier.LevelSuccess,
		Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected notify error: %v", err)
	}

	if received["title"] != "Download Done" {
		t.Errorf("expected title 'Download Done', got %v", received["title"])
	}
}

func TestMultiNotifier(t *testing.T) {
	m1 := notifier.NewMockNotifier()
	m2 := notifier.NewMockNotifier()
	multi := notifier.NewMultiNotifier(m1, m2)

	err := multi.Notify(context.Background(), notifier.Message{
		Title: "Multi Test",
		Body:  "Broadcast",
		Level: notifier.LevelInfo,
	})
	if err != nil {
		t.Fatalf("multi notify failed: %v", err)
	}

	if len(m1.GetMessages()) != 1 || len(m2.GetMessages()) != 1 {
		t.Errorf("expected both notifiers to receive message")
	}
}
