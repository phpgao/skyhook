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

// WebhookNotifier sends notifications to an HTTP/HTTPS Webhook (compatible with standard webhooks and Bark)
type WebhookNotifier struct {
	URL        string
	httpClient *http.Client
}

// NewWebhookNotifier creates a Webhook notifier
func NewWebhookNotifier(url string, client *http.Client) *WebhookNotifier {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &WebhookNotifier{
		URL:        url,
		httpClient: client,
	}
}

func (w *WebhookNotifier) Notify(ctx context.Context, msg Message) error {
	if w.URL == "" {
		return nil
	}

	// Payload suitable for standard Webhook / Bark / Slack / Feishu / WeCom
	payload := map[string]interface{}{
		"title":     msg.Title,
		"body":      msg.Body,
		"level":     string(msg.Level),
		"timestamp": msg.Timestamp.Format(time.RFC3339),
		// Enterprise WeChat / Dingtalk compatible text content
		"msgtype": "text",
		"text": map[string]string{
			"content": fmt.Sprintf("【%s】\n%s", msg.Title, msg.Body),
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook responded with non-2xx status: %d", resp.StatusCode)
	}

	return nil
}

// BarkNotifier adapts specifically to Bark iOS push app
type BarkNotifier struct {
	BaseURL    string
	httpClient *http.Client
}

// NewBarkNotifier creates a Bark push notifier
func NewBarkNotifier(baseURL string, client *http.Client) *BarkNotifier {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &BarkNotifier{
		BaseURL:    baseURL,
		httpClient: client,
	}
}

func (b *BarkNotifier) Notify(ctx context.Context, msg Message) error {
	if b.BaseURL == "" {
		return nil
	}

	payload := map[string]interface{}{
		"title": msg.Title,
		"body":  msg.Body,
		"group": "skyhook",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.BaseURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("bark push failed: %w", err)
	}
	defer resp.Body.Close()

	return nil
}
