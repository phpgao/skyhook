package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// DingTalkNotifier sends notifications to DingTalk (钉钉) robot webhooks
type DingTalkNotifier struct {
	URL        string
	httpClient *http.Client
}

// NewDingTalkNotifier creates a new DingTalk notifier
func NewDingTalkNotifier(url string, client *http.Client) *DingTalkNotifier {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &DingTalkNotifier{
		URL:        url,
		httpClient: client,
	}
}

func (d *DingTalkNotifier) Notify(ctx context.Context, msg Message) error {
	if d.URL == "" {
		return nil
	}

	payload := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": msg.Title,
			"text":  fmt.Sprintf("### %s\n\n%s\n\n> %s", msg.Title, msg.Body, msg.Timestamp.Format("2006-01-02 15:04:05")),
		},
	}

	return postJSON(ctx, d.httpClient, d.URL, payload)
}

// WeComNotifier sends notifications to WeChat Work / Enterprise WeChat (企业微信) robot webhooks
type WeComNotifier struct {
	URL        string
	httpClient *http.Client
}

// NewWeComNotifier creates a new WeCom notifier
func NewWeComNotifier(url string, client *http.Client) *WeComNotifier {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &WeComNotifier{
		URL:        url,
		httpClient: client,
	}
}

func (w *WeComNotifier) Notify(ctx context.Context, msg Message) error {
	if w.URL == "" {
		return nil
	}

	payload := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": fmt.Sprintf("### %s\n%s\n> 时间: %s", msg.Title, msg.Body, msg.Timestamp.Format("2006-01-02 15:04:05")),
		},
	}

	return postJSON(ctx, w.httpClient, w.URL, payload)
}

// FeishuNotifier sends notifications to Feishu / Lark (飞书) robot webhooks
type FeishuNotifier struct {
	URL        string
	httpClient *http.Client
}

// NewFeishuNotifier creates a new Feishu notifier
func NewFeishuNotifier(url string, client *http.Client) *FeishuNotifier {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &FeishuNotifier{
		URL:        url,
		httpClient: client,
	}
}

func (f *FeishuNotifier) Notify(ctx context.Context, msg Message) error {
	if f.URL == "" {
		return nil
	}

	payload := map[string]interface{}{
		"msg_type": "text",
		"content": map[string]string{
			"text": fmt.Sprintf("【%s】\n%s\n(%s)", msg.Title, msg.Body, msg.Timestamp.Format("2006-01-02 15:04:05")),
		},
	}

	return postJSON(ctx, f.httpClient, f.URL, payload)
}

func postJSON(ctx context.Context, client *http.Client, url string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("notification post failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notification target returned status %d", resp.StatusCode)
	}
	return nil
}
