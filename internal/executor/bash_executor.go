package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// BashExecutor executes shell commands using bash or sh
type BashExecutor struct {
	Shell string
}

// NewBashExecutor creates an executor with default shell
func NewBashExecutor() *BashExecutor {
	shell := "bash"
	if _, err := exec.LookPath("bash"); err != nil {
		shell = "sh"
	}
	return &BashExecutor{Shell: shell}
}

func (b *BashExecutor) Execute(ctx context.Context, cmdStr string, env map[string]string) (*CommandResult, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, b.Shell, "-c", cmdStr)

	// Inherit system env and merge custom env
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	out, err := cmd.CombinedOutput()
	duration := time.Since(start)

	result := &CommandResult{
		Output:   string(out),
		Duration: duration,
		ExitCode: 0,
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return result, fmt.Errorf("command execution timed out: %w", ctx.Err())
		}
		if ctx.Err() == context.Canceled {
			return result, fmt.Errorf("command execution canceled: %w", ctx.Err())
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, fmt.Errorf("command exited with code %d: %s", exitErr.ExitCode(), string(out))
		}
		return result, fmt.Errorf("failed to run command: %w (output: %s)", err, string(out))
	}

	return result, nil
}
