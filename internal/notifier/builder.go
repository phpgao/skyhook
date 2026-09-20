package notifier

import (
	"strings"

	"github.com/phpgao/skyhook/internal/config"
)

// BuildFromConfig constructs a composite MultiNotifier from the configuration
func BuildFromConfig(cfg *config.Config) Notifier {
	var notifiers []Notifier

	// 1. Legacy / direct channels
	if cfg.Notify.WebhookURL != "" {
		notifiers = append(notifiers, NewWebhookNotifier(cfg.Notify.WebhookURL, nil))
	}
	if cfg.Notify.BarkURL != "" {
		notifiers = append(notifiers, NewBarkNotifier(cfg.Notify.BarkURL, nil))
	}
	if cfg.Notify.Telegram.BotToken != "" && cfg.Notify.Telegram.ChatID != "" {
		notifiers = append(notifiers, NewTelegramNotifier(cfg.Notify.Telegram.BotToken, cfg.Notify.Telegram.ChatID, nil))
	}

	// 2. Multi-channel array
	for _, ch := range cfg.Notify.Channels {
		if ch.Enabled != nil && !*ch.Enabled {
			continue
		}
		chType := strings.ToLower(strings.TrimSpace(ch.Type))
		switch chType {
		case "telegram":
			if ch.BotToken != "" && ch.ChatID != "" {
				notifiers = append(notifiers, NewTelegramNotifier(ch.BotToken, ch.ChatID, nil))
			}
		case "wecom", "qywx", "wxwork":
			if ch.URL != "" {
				notifiers = append(notifiers, NewWeComNotifier(ch.URL, nil))
			}
		case "dingtalk", "ding":
			if ch.URL != "" {
				notifiers = append(notifiers, NewDingTalkNotifier(ch.URL, nil))
			}
		case "feishu", "lark":
			if ch.URL != "" {
				notifiers = append(notifiers, NewFeishuNotifier(ch.URL, nil))
			}
		case "bark":
			if ch.URL != "" {
				notifiers = append(notifiers, NewBarkNotifier(ch.URL, nil))
			}
		case "webhook", "http", "slack":
			if ch.URL != "" {
				notifiers = append(notifiers, NewWebhookNotifier(ch.URL, nil))
			}
		}
	}

	return NewMultiNotifier(notifiers...)
}
