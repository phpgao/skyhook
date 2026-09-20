package executor_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/phpgao/skyhook/internal/executor"
)

func TestBashExecutor_EchoSuccess(t *testing.T) {
	exec := executor.NewBashExecutor()
	res, err := exec.Execute(context.Background(), "echo 'hello skyhook'", nil)
	if err != nil {
		t.Fatalf("expected command to succeed, got %v", err)
	}
	if !strings.Contains(res.Output, "hello skyhook") {
		t.Errorf("unexpected output: %s", res.Output)
	}
	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}
}

func TestBashExecutor_EnvInjection(t *testing.T) {
	exec := executor.NewBashExecutor()
	env := map[string]string{
		"MY_VAR": "skyhook_test_val",
	}
	res, err := exec.Execute(context.Background(), "echo \"$MY_VAR\"", env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Output, "skyhook_test_val") {
		t.Errorf("expected env var to be injected, got: %s", res.Output)
	}
}

func TestBashExecutor_NonZeroExit(t *testing.T) {
	exec := executor.NewBashExecutor()
	res, err := exec.Execute(context.Background(), "exit 42", nil)
	if err == nil {
		t.Fatalf("expected error from non-zero exit code")
	}
	if res.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", res.ExitCode)
	}
}

func TestBashExecutor_Timeout(t *testing.T) {
	exec := executor.NewBashExecutor()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := exec.Execute(ctx, "sleep 2", nil)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") && !strings.Contains(err.Error(), "deadline exceeded") {
		t.Errorf("expected timeout message, got %v", err)
	}
}
