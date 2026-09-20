package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// TelegramNotifier sends notifications via Telegram Bot API
type TelegramNotifier struct {
	BotToken   string
	ChatID     string
	httpClient *http.Client
}

// NewTelegramNotifier creates a new Telegram notifier
func NewTelegramNotifier(botToken, chatID string, client *http.Client) *TelegramNotifier {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &TelegramNotifier{
		BotToken:   botToken,
		ChatID:     chatID,
		httpClient: client,
	}
}

func (t *TelegramNotifier) Notify(ctx context.Context, msg Message) error {
	if t.BotToken == "" || t.ChatID == "" {
		return nil
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.BotToken)

	icon := "ℹ️"
	switch msg.Level {
	case LevelSuccess:
		icon = "🎉"
	case LevelWarn:
		icon = "⚠️"
	case LevelError:
		icon = "❌"
	}

	text := fmt.Sprintf("%s *%s*\n\n%s\n\n_🕒 %s_",
		icon,
		escapeMarkdown(msg.Title),
		msg.Body,
		time.Now().Format("2006-01-02 15:04:05"),
	)

	payload := map[string]interface{}{
		"chat_id":    t.ChatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal telegram payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram notification failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram api returned status %d", resp.StatusCode)
	}

	return nil
}

func escapeMarkdown(s string) string {
	// Simple escape for Telegram Markdown special characters in titles
	r := stringsNewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"`", "\\`",
	)
	return r.Replace(s)
}

type replacer struct {
	pairs []string
}

func stringsNewReplacer(pairs ...string) *replacer {
	return &replacer{pairs: pairs}
}

func (r *replacer) Replace(s string) string {
	for i := 0; i < len(r.pairs); i += 2 {
		s = replaceAll(s, r.pairs[i], r.pairs[i+1])
	}
	return s
}

func replaceAll(s, old, new string) string {
	var b bytes.Buffer
	for {
		idx := bytes.Index([]byte(s), []byte(old))
		if idx == -1 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:idx])
		b.WriteString(new)
		s = s[idx+len(old):]
	}
	return b.String()
}
