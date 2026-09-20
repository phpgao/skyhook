package executor

import (
	"context"
	"time"
)

// CommandResult holds output and metadata from a command execution
type CommandResult struct {
	Output   string        `json:"output"`
	Duration time.Duration `json:"duration"`
	ExitCode int           `json:"exit_code"`
}

// Executor defines interface for executing shell / system commands
type Executor interface {
	Execute(ctx context.Context, cmdStr string, env map[string]string) (*CommandResult, error)
}
