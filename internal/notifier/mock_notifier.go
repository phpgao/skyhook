package notifier

import (
	"context"
	"sync"
)

// MockNotifier captures all sent messages for test assertions
type MockNotifier struct {
	mu       sync.Mutex
	Messages []Message
	Fail     bool
}

// NewMockNotifier creates a test mock notifier
func NewMockNotifier() *MockNotifier {
	return &MockNotifier{}
}

func (m *MockNotifier) Notify(ctx context.Context, msg Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Fail {
		return context.DeadlineExceeded
	}

	m.Messages = append(m.Messages, msg)
	return nil
}

// GetMessages returns a snapshot of recorded messages
func (m *MockNotifier) GetMessages() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()

	res := make([]Message, len(m.Messages))
	copy(res, m.Messages)
	return res
}
