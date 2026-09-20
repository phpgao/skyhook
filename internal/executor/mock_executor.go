package executor

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MockExecutor allows simulating command executions with custom responses and delays
type MockExecutor struct {
	mu           sync.Mutex
	CallHistory  []string
	Responses    map[string]*CommandResult // key: command prefix or exact string
	Errors       map[string]error
	DefaultError error
	Delay        time.Duration
}

// NewMockExecutor creates a new mock executor
func NewMockExecutor() *MockExecutor {
	return &MockExecutor{
		Responses: make(map[string]*CommandResult),
		Errors:    make(map[string]error),
	}
}

func (m *MockExecutor) Execute(ctx context.Context, cmdStr string, env map[string]string) (*CommandResult, error) {
	m.mu.Lock()
	m.CallHistory = append(m.CallHistory, cmdStr)
	res := m.Responses[cmdStr]
	err := m.Errors[cmdStr]
	defErr := m.DefaultError
	delay := m.Delay
	m.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return &CommandResult{ExitCode: -1}, ctx.Err()
		}
	}

	if err != nil {
		return &CommandResult{ExitCode: 1, Output: err.Error()}, err
	}
	if defErr != nil {
		return &CommandResult{ExitCode: 1, Output: defErr.Error()}, defErr
	}
	if res != nil {
		return res, nil
	}

	return &CommandResult{
		Output:   fmt.Sprintf("mock executed: %s", cmdStr),
		Duration: 10 * time.Millisecond,
		ExitCode: 0,
	}, nil
}
