package hook

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/phpgao/skyhook/internal/executor"
	"github.com/phpgao/skyhook/internal/model"
	"github.com/phpgao/skyhook/internal/notifier"
)

// HookContext provides metadata context to hook commands
type HookContext struct {
	TaskID     string
	URL        string
	Path       string
	Name       string
	Size       int64
	Error      string
	Downloader string
	Progress   float64
	Speed      int64
}

// CommandResult records output from a hook command
type CommandResult struct {
	Name     string        `json:"name"`
	Run      string        `json:"run"`
	Success  bool          `json:"success"`
	Attempts int           `json:"attempts"`
	Duration time.Duration `json:"duration"`
	Output   string        `json:"output,omitempty"`
	ErrorMsg string        `json:"error_msg,omitempty"`
}

// HookResult is the outcome of a hook trigger
type HookResult struct {
	Type           model.HookType  `json:"type"`
	Success        bool            `json:"success"`
	CommandResults []CommandResult `json:"command_results"`
	TotalDuration  time.Duration   `json:"total_duration"`
}

// Engine defines the hook lifecycle execution contract
type Engine interface {
	Trigger(ctx context.Context, hookType model.HookType, hookCfg *model.HookConfig, hCtx HookContext, customNotifier notifier.Notifier) (*HookResult, error)
}

// StandardHookEngine implements Engine
type StandardHookEngine struct {
	executor       executor.Executor
	globalNotifier notifier.Notifier
}

// NewStandardHookEngine creates a new hook execution engine
func NewStandardHookEngine(exec executor.Executor, notify notifier.Notifier) *StandardHookEngine {
	return &StandardHookEngine{
		executor:       exec,
		globalNotifier: notify,
	}
}

func (h *StandardHookEngine) Trigger(
	ctx context.Context,
	hookType model.HookType,
	hookCfg *model.HookConfig,
	hCtx HookContext,
	customNotifier notifier.Notifier,
) (*HookResult, error) {
	start := time.Now()

	// Composite notifier
	var notify notifier.Notifier
	if customNotifier != nil && h.globalNotifier != nil {
		notify = notifier.NewMultiNotifier(h.globalNotifier, customNotifier)
	} else if customNotifier != nil {
		notify = customNotifier
	} else {
		notify = h.globalNotifier
	}

	result := &HookResult{
		Type:    hookType,
		Success: true,
	}

	if hookCfg == nil || len(hookCfg.Commands) == 0 {
		// Even without commands, if notify is enabled, send hook event notification
		if hookCfg != nil && hookCfg.Notify && notify != nil {
			h.sendHookEventNotification(ctx, notify, hookType, hCtx, nil)
		}
		result.TotalDuration = time.Since(start)
		return result, nil
	}

	for _, cmd := range hookCfg.Commands {
		cmdRes := h.runCommandWithRetry(ctx, cmd, hCtx, notify)
		result.CommandResults = append(result.CommandResults, cmdRes)

		if !cmdRes.Success {
			result.Success = false
			if !cmd.ContinueOnError {
				break
			}
		}
	}

	result.TotalDuration = time.Since(start)

	// Send hook notification if enabled
	if hookCfg.Notify && notify != nil {
		h.sendHookEventNotification(ctx, notify, hookType, hCtx, result)
	}

	return result, nil
}

func (h *StandardHookEngine) runCommandWithRetry(
	ctx context.Context,
	cmd model.HookCommand,
	hCtx HookContext,
	notify notifier.Notifier,
) CommandResult {
	cmdStr := h.interpolate(cmd.Run, hCtx)

	maxAttempts := cmd.Retries + 1
	var lastErr error
	var lastOutput string
	cmdStart := time.Now()

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		cmdTimeout := cmd.Timeout
		if cmdTimeout <= 0 {
			cmdTimeout = 30 * time.Minute
		}

		cmdCtx, cancel := context.WithTimeout(ctx, cmdTimeout)
		res, err := h.executor.Execute(cmdCtx, cmdStr, map[string]string{
			"SKYHOOK_TASK_ID":    hCtx.TaskID,
			"SKYHOOK_PATH":       hCtx.Path,
			"SKYHOOK_NAME":       hCtx.Name,
			"SKYHOOK_URL":        hCtx.URL,
			"SKYHOOK_ERROR":      hCtx.Error,
			"SKYHOOK_DOWNLOADER": hCtx.Downloader,
			"SKYHOOK_SIZE":       fmt.Sprintf("%d", hCtx.Size),
		})
		cancel()

		if res != nil {
			lastOutput = res.Output
		}

		if err == nil {
			return CommandResult{
				Name:     cmd.Name,
				Run:      cmdStr,
				Success:  true,
				Attempts: attempt,
				Duration: time.Since(cmdStart),
				Output:   lastOutput,
			}
		}

		lastErr = err

		if attempt < maxAttempts {
			if notify != nil {
				_ = notify.Notify(ctx, notifier.Message{
					Title:     fmt.Sprintf("Hook 命令重试 [%s]", cmd.Name),
					Body:      fmt.Sprintf("命令执行失败: %v\n将在 %s 后进行第 %d 次重试...", lastErr, cmd.RetryInterval, attempt+1),
					Level:     notifier.LevelWarn,
					Timestamp: time.Now(),
				})
			}

			select {
			case <-time.After(cmd.RetryInterval):
			case <-ctx.Done():
				return CommandResult{
					Name:     cmd.Name,
					Run:      cmdStr,
					Success:  false,
					Attempts: attempt,
					Duration: time.Since(cmdStart),
					ErrorMsg: ctx.Err().Error(),
				}
			}
		}
	}

	return CommandResult{
		Name:     cmd.Name,
		Run:      cmdStr,
		Success:  false,
		Attempts: maxAttempts,
		Duration: time.Since(cmdStart),
		ErrorMsg: lastErr.Error(),
		Output:   lastOutput,
	}
}

func (h *StandardHookEngine) interpolate(template string, hCtx HookContext) string {
	s := strings.ReplaceAll(template, "{path}", hCtx.Path)
	s = strings.ReplaceAll(s, "{name}", hCtx.Name)
	s = strings.ReplaceAll(s, "{url}", hCtx.URL)
	s = strings.ReplaceAll(s, "{id}", hCtx.TaskID)
	s = strings.ReplaceAll(s, "{error}", hCtx.Error)
	s = strings.ReplaceAll(s, "{downloader}", hCtx.Downloader)
	s = strings.ReplaceAll(s, "{size}", fmt.Sprintf("%d", hCtx.Size))
	return s
}

func (h *StandardHookEngine) sendHookEventNotification(
	ctx context.Context,
	notify notifier.Notifier,
	hookType model.HookType,
	hCtx HookContext,
	res *HookResult,
) {
	var title string
	var level notifier.MessageLevel
	var sb strings.Builder

	switch hookType {
	case model.HookOnCreateFailed:
		title = "❌ 任务创建失败 [Hook: on_create_failed]"
		level = notifier.LevelError
		sb.WriteString(fmt.Sprintf("URL: %s\n失败原因: %s\n", hCtx.URL, hCtx.Error))

	case model.HookOnStart:
		title = "🚀 下载任务开始 [Hook: on_start]"
		level = notifier.LevelInfo
		sb.WriteString(fmt.Sprintf("任务 ID: %s\n下载器: %s\nURL: %s\n", hCtx.TaskID, hCtx.Downloader, hCtx.URL))

	case model.HookOnProgress:
		title = "⏳ 任务下载巡检进度 [Hook: on_progress]"
		level = notifier.LevelInfo
		sb.WriteString(fmt.Sprintf("任务 ID: %s\n下载器: %s\n当前进度: %.1f%%\n瞬时速度: %s/s\n",
			hCtx.TaskID, hCtx.Downloader, hCtx.Progress, formatBytes(hCtx.Speed)))

	case model.HookOnComplete:
		level = notifier.LevelSuccess
		title = "🎉 下载完成 [Hook: on_complete]"
		sb.WriteString(fmt.Sprintf("任务 ID: %s\n文件: %s\n路径: %s\n大小: %d 字节\n", hCtx.TaskID, hCtx.Name, hCtx.Path, hCtx.Size))
		if res != nil && len(res.CommandResults) > 0 {
			sb.WriteString("\n命令执行结果:\n")
			for i, cr := range res.CommandResults {
				icon := "✅"
				if !cr.Success {
					icon = "❌"
					title = "⚠️ 下载完成但转存Hook部分失败"
					level = notifier.LevelWarn
				}
				sb.WriteString(fmt.Sprintf("%d. %s %s (%s, 尝试 %d 次)\n", i+1, icon, cr.Name, cr.Duration.Round(time.Millisecond), cr.Attempts))
				if !cr.Success && cr.ErrorMsg != "" {
					sb.WriteString(fmt.Sprintf("   错误: %s\n", cr.ErrorMsg))
				}
			}
		}

	case model.HookOnError:
		title = "❌ 任务异常终止 [Hook: on_error]"
		level = notifier.LevelError
		sb.WriteString(fmt.Sprintf("任务 ID: %s\n错误原因: %s\nURL: %s\n", hCtx.TaskID, hCtx.Error, hCtx.URL))
	}

	_ = notify.Notify(ctx, notifier.Message{
		Title:     title,
		Body:      sb.String(),
		Level:     level,
		Timestamp: time.Now(),
	})
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
