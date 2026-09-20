package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/phpgao/skyhook/internal/model"
	"github.com/phpgao/skyhook/internal/storage"
)

// Config represents the top-level server configuration
type Config struct {
	Server        ServerConfig            `yaml:"server"`
	Storage       StorageConfig           `yaml:"storage"`
	Aria2         Aria2Config             `yaml:"aria2"`
	Downloaders   []DownloaderConfig      `yaml:"downloaders"`
	Limits        LimitsConfig            `yaml:"limits"`
	DefaultAction string                  `yaml:"default_action"`
	Actions       map[string]model.Action `yaml:"actions"`
	GlobalHooks   model.Hooks             `yaml:"hooks"`
	Notify        NotifyConfig            `yaml:"notify"`
}

type StorageConfig struct {
	DownloadDir  string `yaml:"download_dir"`   // Download directory path (e.g. /data/downloads)
	MinFreeSpace string `yaml:"min_free_space"` // Minimum free space to reserve (e.g. "2GB", "500MB")
	StateFile    string `yaml:"state_file"`     // Path to state persistence file (default: <download_dir>/.skyhook_tasks.json)
}

type DownloaderConfig struct {
	Name        string   `yaml:"name"`
	Type        string   `yaml:"type"` // "cli", "aria2_rpc", "native"
	Bin         string   `yaml:"bin"`  // Executable path or command name in PATH
	Enabled     *bool    `yaml:"enabled"`
	Priority    int      `yaml:"priority"` // 0 to 99 (99 is highest)
	TargetTypes []string `yaml:"target_types"`
	Domains     []string `yaml:"domains"`
	Args        []string `yaml:"args"`
	RPCURL      string   `yaml:"rpc_url"`
	RPCSecret   string   `yaml:"rpc_secret"`
}

type ServerConfig struct {
	Port      int    `yaml:"port"`
	AuthToken string `yaml:"auth_token"`
}

type Aria2Config struct {
	RPCURL      string `yaml:"rpc_url"`
	RPCSecret   string `yaml:"rpc_secret"`
	DownloadDir string `yaml:"download_dir"`
	Trackers    string `yaml:"trackers"`
}

type LimitsConfig struct {
	DefaultDownloadLimit string        `yaml:"default_download_limit"`
	DefaultUploadLimit   string        `yaml:"default_upload_limit"`
	DownloadTimeout      time.Duration `yaml:"download_timeout"`
}

type TelegramConfig struct {
	BotToken string `yaml:"bot_token"`
	ChatID   string `yaml:"chat_id"`
}

type ChannelConfig struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"` // "telegram", "wecom", "dingtalk", "feishu", "bark", "webhook"
	Enabled  *bool  `yaml:"enabled"`
	URL      string `yaml:"url"`       // Webhook endpoint (for wecom, dingtalk, feishu, bark, webhook)
	BotToken string `yaml:"bot_token"` // For telegram
	ChatID   string `yaml:"chat_id"`   // For telegram
	Secret   string `yaml:"secret"`    // Optional sign secret for DingTalk / Feishu
}

type NotifyConfig struct {
	WebhookURL string          `yaml:"webhook_url"`
	BarkURL    string          `yaml:"bark_url"`
	Telegram   TelegramConfig  `yaml:"telegram"`
	Channels   []ChannelConfig `yaml:"channels"`
}

// Load reads and parses the configuration from a YAML file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
	}
	return LoadFromBytes(data)
}

// LoadFromBytes parses configuration from YAML byte slice
func LoadFromBytes(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML configuration: %w", err)
	}

	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Aria2.RPCURL == "" {
		c.Aria2.RPCURL = "http://127.0.0.1:6800/jsonrpc"
	}
	if c.Storage.DownloadDir == "" {
		if c.Aria2.DownloadDir != "" {
			c.Storage.DownloadDir = c.Aria2.DownloadDir
		} else {
			c.Storage.DownloadDir = "/tmp/skyhook_downloads"
		}
	}
	if c.Aria2.DownloadDir == "" {
		c.Aria2.DownloadDir = c.Storage.DownloadDir
	}
	if c.Storage.MinFreeSpace == "" {
		c.Storage.MinFreeSpace = "1GB"
	}
	if c.Limits.DownloadTimeout == 0 {
		c.Limits.DownloadTimeout = 2 * time.Hour
	}
	if c.Limits.DefaultUploadLimit == "" {
		c.Limits.DefaultUploadLimit = "1M"
	}
	if c.Limits.DefaultDownloadLimit == "" {
		c.Limits.DefaultDownloadLimit = "0"
	}

	// Ensure action names and hooks are populated from map keys if empty
	for key, action := range c.Actions {
		if action.Name == "" {
			action.Name = key
		}

		// Backward compatibility: map action.Steps into action.Hooks.OnComplete
		if action.Hooks.OnComplete == nil && len(action.Steps) > 0 {
			var cmds []model.HookCommand
			for _, step := range action.Steps {
				to := step.Timeout
				if to == 0 {
					to = 30 * time.Minute
				}
				ri := step.RetryInterval
				if ri == 0 {
					ri = 5 * time.Second
				}
				cmds = append(cmds, model.HookCommand{
					Name:            step.Name,
					Run:             step.Command,
					Timeout:         to,
					Retries:         step.Retries,
					RetryInterval:   ri,
					ContinueOnError: step.ContinueOnError,
				})
			}
			action.Hooks.OnComplete = &model.HookConfig{
				Notify:   true,
				Commands: cmds,
			}
		}

		// Apply defaults to Hook Commands
		if action.Hooks.OnStart != nil {
			for i := range action.Hooks.OnStart.Commands {
				if action.Hooks.OnStart.Commands[i].Timeout == 0 {
					action.Hooks.OnStart.Commands[i].Timeout = 5 * time.Minute
				}
			}
		}
		if action.Hooks.OnComplete != nil {
			for i := range action.Hooks.OnComplete.Commands {
				if action.Hooks.OnComplete.Commands[i].Timeout == 0 {
					action.Hooks.OnComplete.Commands[i].Timeout = 30 * time.Minute
				}
				if action.Hooks.OnComplete.Commands[i].RetryInterval == 0 {
					action.Hooks.OnComplete.Commands[i].RetryInterval = 5 * time.Second
				}
			}
		}
		if action.Hooks.OnError != nil {
			for i := range action.Hooks.OnError.Commands {
				if action.Hooks.OnError.Commands[i].Timeout == 0 {
					action.Hooks.OnError.Commands[i].Timeout = 5 * time.Minute
				}
			}
		}

		c.Actions[key] = action
	}
}

func (c *Config) validate() error {
	if len(c.Actions) > 0 && c.DefaultAction != "" {
		if _, ok := c.Actions[c.DefaultAction]; !ok {
			return fmt.Errorf("default_action %q not found in defined actions", c.DefaultAction)
		}
	}
	if _, err := storage.ParseBytes(c.Storage.MinFreeSpace); err != nil {
		return fmt.Errorf("invalid storage.min_free_space %q: %w", c.Storage.MinFreeSpace, err)
	}
	return nil
}

// GetDownloadDir returns the resolved global download directory
func (c *Config) GetDownloadDir() string {
	if c.Storage.DownloadDir != "" {
		return c.Storage.DownloadDir
	}
	if c.Aria2.DownloadDir != "" {
		return c.Aria2.DownloadDir
	}
	return "/tmp/skyhook_downloads"
}

// GetMinFreeSpaceBytes returns the minimum free space to reserve in bytes
func (c *Config) GetMinFreeSpaceBytes() uint64 {
	b, err := storage.ParseBytes(c.Storage.MinFreeSpace)
	if err != nil {
		return 1024 * 1024 * 1024 // 1GB default fallback
	}
	return b
}

// GetStateFile returns the path where task state will be persisted
func (c *Config) GetStateFile() string {
	if c.Storage.StateFile != "" {
		return c.Storage.StateFile
	}
	return filepath.Join(c.GetDownloadDir(), ".skyhook_tasks.json")
}

// GetAction resolves an action by name, or returns the default action if name is empty
func (c *Config) GetAction(name string) (*model.Action, error) {
	targetName := name
	if targetName == "" {
		targetName = c.DefaultAction
	}

	if targetName == "" {
		// No action configured and none requested
		return &model.Action{Name: "noop"}, nil
	}

	action, ok := c.Actions[targetName]
	if !ok {
		return nil, fmt.Errorf("action %q not found in server configuration", targetName)
	}

	return &action, nil
}
