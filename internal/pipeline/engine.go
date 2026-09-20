package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/phpgao/skyhook/internal/executor"
	"github.com/phpgao/skyhook/internal/model"
	"github.com/phpgao/skyhook/internal/notifier"
)

// Engine defines the contract for running post-download pipelines
type Engine interface {
	Execute(ctx context.Context, targetPath string, action model.Action, customNotifier notifier.Notifier) (*model.PipelineResult, error)
}

// StandardEngine executes pipeline steps sequentially using an Executor and Notifier
type StandardEngine struct {
	executor       executor.Executor
	globalNotifier notifier.Notifier
}

// NewStandardEngine creates a new pipeline engine
func NewStandardEngine(exec executor.Executor, notify notifier.Notifier) *StandardEngine {
	return &StandardEngine{
		executor:       exec,
		globalNotifier: notify,
	}
}

func (e *StandardEngine) Execute(ctx context.Context, targetPath string, action model.Action, customNotifier notifier.Notifier) (*model.PipelineResult, error) {
	start := time.Now()
	fileName := filepath.Base(targetPath)

	// Combine global notifier with any custom per-task notifier
	var notify notifier.Notifier
	if customNotifier != nil && e.globalNotifier != nil {
		notify = notifier.NewMultiNotifier(e.globalNotifier, customNotifier)
	} else if customNotifier != nil {
		notify = customNotifier
	} else {
		notify = e.globalNotifier
	}

	if notify != nil {
		_ = notify.Notify(ctx, notifier.Message{
			Title:     "流水线启动",
			Body:      fmt.Sprintf("文件: %s\n开始执行动作流程: [%s]，共 %d 个步骤", fileName, action.Name, len(action.Steps)),
			Level:     notifier.LevelInfo,
			Timestamp: time.Now(),
		})
	}

	result := &model.PipelineResult{
		ActionName: action.Name,
		TargetPath: targetPath,
		Success:    true,
	}

	// Execute steps sequentially
	for idx, step := range action.Steps {
		stepRes := e.runStepWithRetry(ctx, targetPath, fileName, step, idx+1, len(action.Steps), notify)
		result.StepResults = append(result.StepResults, stepRes)

		if !stepRes.Success {
			result.Success = false
			if !step.ContinueOnError {
				if notify != nil {
					_ = notify.Notify(ctx, notifier.Message{
						Title:     "流水线中断",
						Body:      fmt.Sprintf("步骤 [%s] 执行失败且未开启容错，流程强制终止！\n错误: %s", step.Name, stepRes.ErrorMsg),
						Level:     notifier.LevelError,
						Timestamp: time.Now(),
					})
				}
				break
			}
		}
	}

	result.TotalDuration = time.Since(start)

	// Clean up VPS disk
	shouldClean := false
	if action.CleanAtEnd {
		shouldClean = true
	} else if !result.Success && action.CleanOnFail {
		shouldClean = true
	}

	if shouldClean && targetPath != "" {
		_ = os.RemoveAll(targetPath)
	}

	// Send final summary notification
	if notify != nil {
		e.sendSummary(ctx, notify, result, fileName)
	}

	return result, nil
}

func (e *StandardEngine) runStepWithRetry(
	ctx context.Context,
	targetPath, fileName string,
	step model.Step,
	stepIdx, totalSteps int,
	notify notifier.Notifier,
) model.StepResult {
	cmdStr := strings.ReplaceAll(step.Command, "{path}", targetPath)
	cmdStr = strings.ReplaceAll(cmdStr, "{name}", fileName)

	maxAttempts := step.Retries + 1
	var lastErr error
	var lastOutput string
	stepStart := time.Now()

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Enforce step timeout
		stepTimeout := step.Timeout
		if stepTimeout <= 0 {
			stepTimeout = 30 * time.Minute
		}

		stepCtx, cancel := context.WithTimeout(ctx, stepTimeout)
		res, err := e.executor.Execute(stepCtx, cmdStr, map[string]string{
			"SKYHOOK_PATH": targetPath,
			"SKYHOOK_NAME": fileName,
		})
		cancel()

		if res != nil {
			lastOutput = res.Output
		}

		if err == nil {
			return model.StepResult{
				StepName: step.Name,
				Success:  true,
				Attempts: attempt,
				Duration: time.Since(stepStart),
				Output:   lastOutput,
			}
		}

		lastErr = err

		// Check if we can retry
		if attempt < maxAttempts {
			if notify != nil {
				_ = notify.Notify(ctx, notifier.Message{
					Title:     "步骤重试提醒",
					Body:      fmt.Sprintf("步骤 [%s] 第 %d 次尝试失败: %v\n将在 %s 后进行第 %d 次重试...", step.Name, attempt, lastErr, step.RetryInterval, attempt+1),
					Level:     notifier.LevelWarn,
					Timestamp: time.Now(),
				})
			}

			select {
			case <-time.After(step.RetryInterval):
			case <-ctx.Done():
				return model.StepResult{
					StepName: step.Name,
					Success:  false,
					Attempts: attempt,
					Duration: time.Since(stepStart),
					ErrorMsg: ctx.Err().Error(),
				}
			}
		}
	}

	// All attempts failed
	if notify != nil {
		_ = notify.Notify(ctx, notifier.Message{
			Title:     fmt.Sprintf("步骤 [%s] 最终失败", step.Name),
			Body:      fmt.Sprintf("尝试 %d 次后失败: %v\n继续后续步骤: %v", maxAttempts, lastErr, step.ContinueOnError),
			Level:     notifier.LevelError,
			Timestamp: time.Now(),
		})
	}

	return model.StepResult{
		StepName: step.Name,
		Success:  false,
		Attempts: maxAttempts,
		Duration: time.Since(stepStart),
		ErrorMsg: lastErr.Error(),
		Output:   lastOutput,
	}
}

func (e *StandardEngine) sendSummary(ctx context.Context, notify notifier.Notifier, res *model.PipelineResult, fileName string) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("动作流程: [%s]\n文件: %s\n总耗时: %s\n\n步骤明细:\n", res.ActionName, fileName, res.TotalDuration.Round(time.Second)))

	for i, s := range res.StepResults {
		icon := "✅"
		if !s.Success {
			icon = "❌"
		}
		sb.WriteString(fmt.Sprintf("%d. %s %s (尝试 %d 次, 耗时 %s)\n", i+1, icon, s.StepName, s.Attempts, s.Duration.Round(time.Millisecond)))
		if !s.Success && s.ErrorMsg != "" {
			sb.WriteString(fmt.Sprintf("   原因: %s\n", s.ErrorMsg))
		}
	}

	title := "🎉 流水线执行全部成功"
	level := notifier.LevelSuccess
	if !res.Success {
		title = "⚠️ 流水线执行包含失败"
		level = notifier.LevelWarn
	}

	_ = notify.Notify(ctx, notifier.Message{
		Title:     title,
		Body:      sb.String(),
		Level:     level,
		Timestamp: time.Now(),
	})
}
