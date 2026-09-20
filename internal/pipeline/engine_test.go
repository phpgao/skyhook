package pipeline_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/phpgao/skyhook/internal/executor"
	"github.com/phpgao/skyhook/internal/model"
	"github.com/phpgao/skyhook/internal/notifier"
	"github.com/phpgao/skyhook/internal/pipeline"
)

func TestEngine_Success(t *testing.T) {
	mockExec := executor.NewMockExecutor()
	mockNotify := notifier.NewMockNotifier()
	engine := pipeline.NewStandardEngine(mockExec, mockNotify)

	action := model.Action{
		Name: "test-pipeline",
		Steps: []model.Step{
			{Name: "step1", Command: "echo 1", Timeout: 1 * time.Second},
			{Name: "step2", Command: "echo 2", Timeout: 1 * time.Second},
		},
	}

	res, err := engine.Execute(context.Background(), "/tmp/test.iso", action, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Success {
		t.Errorf("expected pipeline to succeed")
	}
	if len(res.StepResults) != 2 {
		t.Fatalf("expected 2 step results, got %d", len(res.StepResults))
	}
	if res.StepResults[0].Attempts != 1 || res.StepResults[1].Attempts != 1 {
		t.Errorf("expected 1 attempt per step")
	}

	// Verify notifications received (start + summary)
	msgs := mockNotify.GetMessages()
	if len(msgs) < 2 {
		t.Errorf("expected at least 2 notifications, got %d", len(msgs))
	}
}

func TestEngine_RetryThenSuccess(t *testing.T) {
	mockNotify := notifier.NewMockNotifier()

	action := model.Action{
		Name: "retry-pipeline",
		Steps: []model.Step{
			{
				Name:            "flaky-step",
				Command:         "upload",
				Timeout:         1 * time.Second,
				Retries:         2,
				RetryInterval:   10 * time.Millisecond,
				ContinueOnError: false,
			},
		},
	}

	// Set dynamic error behavior
	go func() {
		// MockExecutor records history
	}()

	// We can set default error on mockExec then clear it, or use custom executor
	flakyExec := &flakyExecutor{failUntil: 2}
	engineFlaky := pipeline.NewStandardEngine(flakyExec, mockNotify)

	res, err := engineFlaky.Execute(context.Background(), "/tmp/file.txt", action, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Success {
		t.Errorf("expected pipeline to eventually succeed via retry")
	}
	if res.StepResults[0].Attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", res.StepResults[0].Attempts)
	}
}

type flakyExecutor struct {
	calls     int
	failUntil int
}

func (f *flakyExecutor) Execute(ctx context.Context, cmdStr string, env map[string]string) (*executor.CommandResult, error) {
	f.calls++
	if f.calls < f.failUntil {
		return &executor.CommandResult{ExitCode: 1, Output: "network glitch"}, fmt.Errorf("network glitch")
	}
	return &executor.CommandResult{ExitCode: 0, Output: "ok"}, nil
}

func TestEngine_ContinueOnError(t *testing.T) {
	mockNotify := notifier.NewMockNotifier()

	action := model.Action{
		Name: "fault-tolerant-pipeline",
		Steps: []model.Step{
			{
				Name:            "failing-step",
				Command:         "fail",
				Timeout:         500 * time.Millisecond,
				Retries:         1,
				RetryInterval:   5 * time.Millisecond,
				ContinueOnError: true, // Should continue!
			},
			{
				Name:            "second-step",
				Command:         "ok",
				Timeout:         500 * time.Millisecond,
				Retries:         0,
				ContinueOnError: false,
			},
		},
	}

	// Make second step succeed by switching fail behavior
	customExec := &stepAwareExecutor{}
	engine2 := pipeline.NewStandardEngine(customExec, mockNotify)

	res, err := engine2.Execute(context.Background(), "/tmp/file.txt", action, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Overall pipeline marked false because step 1 failed, but step 2 executed!
	if res.Success {
		t.Errorf("expected overall pipeline to be marked false")
	}
	if len(res.StepResults) != 2 {
		t.Fatalf("expected 2 steps executed because continue_on_error=true, got %d", len(res.StepResults))
	}
	if res.StepResults[0].Success != false {
		t.Errorf("expected step 1 to fail")
	}
	if res.StepResults[1].Success != true {
		t.Errorf("expected step 2 to succeed")
	}
}

type stepAwareExecutor struct{}

func (s *stepAwareExecutor) Execute(ctx context.Context, cmdStr string, env map[string]string) (*executor.CommandResult, error) {
	if cmdStr == "fail" {
		return &executor.CommandResult{ExitCode: 1, Output: "step failed"}, fmt.Errorf("step failed")
	}
	return &executor.CommandResult{ExitCode: 0, Output: "step ok"}, nil
}

func TestEngine_CleanAtEnd(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test_download.zip")
	if err := os.WriteFile(testFile, []byte("data"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	mockExec := executor.NewMockExecutor()
	mockNotify := notifier.NewMockNotifier()
	engine := pipeline.NewStandardEngine(mockExec, mockNotify)

	action := model.Action{
		Name:       "cleanup-action",
		CleanAtEnd: true,
		Steps: []model.Step{
			{Name: "step", Command: "echo done", Timeout: 1 * time.Second},
		},
	}

	res, err := engine.Execute(context.Background(), testFile, action, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Errorf("expected pipeline to succeed")
	}

	// Verify file was deleted by CleanAtEnd
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Errorf("expected file %s to be removed by CleanAtEnd", testFile)
	}
}
