package notifier_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/phpgao/skyhook/internal/notifier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGotifyNotifier_Success(t *testing.T) {
	var capturedHeader string
	var capturedBody map[string]interface{}
	var capturedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedHeader = r.Header.Get("X-Gotify-Key")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": 1, "appid": 1, "message": "ok"}`))
	}))
	defer server.Close()

	gn := notifier.NewGotifyNotifier(server.URL, "test-app-token", 5, server.Client())

	msg := notifier.Message{
		Title:     "SkyHook Download Finished",
		Body:      "ubuntu.iso downloaded successfully",
		Level:     notifier.LevelSuccess,
		Timestamp: time.Now(),
	}

	err := gn.Notify(context.Background(), msg)
	require.NoError(t, err)

	assert.Equal(t, "/message", capturedPath)
	assert.Equal(t, "test-app-token", capturedHeader)
	assert.Equal(t, "SkyHook Download Finished", capturedBody["title"])
	assert.Equal(t, "ubuntu.iso downloaded successfully", capturedBody["message"])
	assert.Equal(t, float64(5), capturedBody["priority"])
}

func TestGotifyNotifier_PriorityEscalation(t *testing.T) {
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	gn := notifier.NewGotifyNotifier(server.URL, "token", 3, server.Client())

	errMsg := notifier.Message{
		Title: "Disk Space Exhausted",
		Body:  "Space below reserved limit",
		Level: notifier.LevelError,
	}

	err := gn.Notify(context.Background(), errMsg)
	require.NoError(t, err)
	assert.Equal(t, float64(8), capturedBody["priority"])
}
