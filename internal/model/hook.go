package model

import "time"

// HookType defines lifecycle points for hooks
type HookType string

const (
	HookOnCreateFailed HookType = "on_create_failed"
	HookOnStart        HookType = "on_start"
	HookOnProgress     HookType = "on_progress"
	HookOnComplete     HookType = "on_complete"
	HookOnError        HookType = "on_error"
)

// HookCommand represents a single shell command inside a hook
type HookCommand struct {
	Name            string        `yaml:"name" json:"name"`
	Run             string        `yaml:"run" json:"run"`
	Timeout         time.Duration `yaml:"timeout" json:"timeout"`
	Retries         int           `yaml:"retries" json:"retries"`
	RetryInterval   time.Duration `yaml:"retry_interval" json:"retry_interval"`
	ContinueOnError bool          `yaml:"continue_on_error" json:"continue_on_error"`
}

// HookConfig holds commands and notification settings for a specific lifecycle event
type HookConfig struct {
	Notify   bool          `yaml:"notify" json:"notify"`
	Commands []HookCommand `yaml:"commands" json:"commands"`
}

// Hooks defines the full set of lifecycle hooks
type Hooks struct {
	OnCreateFailed *HookConfig `yaml:"on_create_failed" json:"on_create_failed"`
	OnStart        *HookConfig `yaml:"on_start" json:"on_start"`
	OnProgress     *HookConfig `yaml:"on_progress" json:"on_progress"`
	OnComplete     *HookConfig `yaml:"on_complete" json:"on_complete"`
	OnError        *HookConfig `yaml:"on_error" json:"on_error"`
}
