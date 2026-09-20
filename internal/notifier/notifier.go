package notifier

import (
	"context"
	"time"
)

// MessageLevel represents severity of notification
type MessageLevel string

const (
	LevelInfo    MessageLevel = "info"
	LevelSuccess MessageLevel = "success"
	LevelWarn    MessageLevel = "warn"
	LevelError   MessageLevel = "error"
)

// Message is the standard payload sent to notification channels
type Message struct {
	Title     string       `json:"title"`
	Body      string       `json:"body"`
	Level     MessageLevel `json:"level"`
	Timestamp time.Time    `json:"timestamp"`
}

// Notifier defines the interface for notification providers
type Notifier interface {
	Notify(ctx context.Context, msg Message) error
}

// MultiNotifier dispatches notifications to multiple notifiers concurrently
type MultiNotifier struct {
	notifiers []Notifier
}

// NewMultiNotifier creates a composite notifier
func NewMultiNotifier(notifiers ...Notifier) *MultiNotifier {
	var valid []Notifier
	for _, n := range notifiers {
		if n != nil {
			valid = append(valid, n)
		}
	}
	return &MultiNotifier{notifiers: valid}
}

func (m *MultiNotifier) Notify(ctx context.Context, msg Message) error {
	var firstErr error
	for _, n := range m.notifiers {
		if err := n.Notify(ctx, msg); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
