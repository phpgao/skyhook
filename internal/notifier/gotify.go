package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// GotifyNotifier sends push notifications to a self-hosted Gotify server
type GotifyNotifier struct {
	BaseURL    string
	Token      string
	Priority   int
	httpClient *http.Client
}

// NewGotifyNotifier creates a new Gotify push notifier
func NewGotifyNotifier(serverURL, token string, priority int, client *http.Client) *GotifyNotifier {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	serverURL = strings.TrimRight(serverURL, "/")
	if priority <= 0 {
		priority = 5
	}
	return &GotifyNotifier{
		BaseURL:    serverURL,
		Token:      token,
		Priority:   priority,
		httpClient: client,
	}
}

func (g *GotifyNotifier) Notify(ctx context.Context, msg Message) error {
	if g.BaseURL == "" {
		return nil
	}

	// Dynamic priority escalation based on level
	prio := g.Priority
	switch msg.Level {
	case LevelError:
		if prio < 8 {
			prio = 8
		}
	case LevelWarn:
		if prio < 6 {
			prio = 6
		}
	}

	payload := map[string]interface{}{
		"title":    msg.Title,
		"message":  msg.Body,
		"priority": prio,
		"extras": map[string]interface{}{
			"client::display": map[string]string{
				"contentType": "text/markdown",
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal gotify payload: %w", err)
	}

	targetURL := g.BaseURL
	// Automatically append /message if omitted
	if !strings.HasSuffix(targetURL, "/message") {
		targetURL = targetURL + "/message"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to build gotify request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if g.Token != "" {
		req.Header.Set("X-Gotify-Key", g.Token)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gotify request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gotify server returned HTTP status %d", resp.StatusCode)
	}

	return nil
}
