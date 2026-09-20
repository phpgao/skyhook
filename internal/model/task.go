package model

import "time"

// TaskType defines the type of download task
type TaskType string

const (
	TaskTypeURI     TaskType = "uri"
	TaskTypeTorrent TaskType = "torrent"
)

// TaskStatus defines current status of a task
type TaskStatus string

const (
	StatusPending     TaskStatus = "pending"
	StatusDownloading TaskStatus = "downloading"
	StatusProcessing  TaskStatus = "processing"
	StatusCompleted   TaskStatus = "completed"
	StatusFailed      TaskStatus = "failed"
	StatusCanceled    TaskStatus = "canceled"
)

// TaskRecord holds full state of a tracked task for memory and persistence
type TaskRecord struct {
	GID       string     `json:"gid"`
	URL       string     `json:"url"`
	Status    TaskStatus `json:"status"`
	Action    Action     `json:"action"`
	NotifyURL string     `json:"notify_url,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at,omitempty"`
	ErrorMsg  string     `json:"error_msg,omitempty"`
}

// Step represents a single post-download execution step
type Step struct {
	Name            string        `yaml:"name" json:"name"`
	Command         string        `yaml:"command" json:"command"`
	Timeout         time.Duration `yaml:"timeout" json:"timeout"`
	Retries         int           `yaml:"retries" json:"retries"`
	RetryInterval   time.Duration `yaml:"retry_interval" json:"retry_interval"`
	ContinueOnError bool          `yaml:"continue_on_error" json:"continue_on_error"`
}

// Action represents a named pipeline with lifecycle hooks
type Action struct {
	Name        string `yaml:"name" json:"name"`
	CleanAtEnd  bool   `yaml:"clean_at_end" json:"clean_at_end"`
	CleanOnFail bool   `yaml:"clean_on_fail" json:"clean_on_fail"`
	Hooks       Hooks  `yaml:"hooks" json:"hooks"`
	Steps       []Step `yaml:"steps,omitempty" json:"steps,omitempty"` // legacy steps support
}

// StepResult represents the execution outcome of a single step
type StepResult struct {
	StepName string        `json:"step_name"`
	Success  bool          `json:"success"`
	Attempts int           `json:"attempts"`
	Duration time.Duration `json:"duration"`
	Output   string        `json:"output,omitempty"`
	ErrorMsg string        `json:"error_msg,omitempty"`
}

// PipelineResult represents the summary of an action pipeline run
type PipelineResult struct {
	ActionName    string        `json:"action_name"`
	TargetPath    string        `json:"target_path"`
	Success       bool          `json:"success"`
	StepResults   []StepResult  `json:"step_results"`
	TotalDuration time.Duration `json:"total_duration"`
}

// TaskRequest is the payload sent from Client to Server
type TaskRequest struct {
	URLs          []string `json:"urls,omitempty"`           // Multiple URLs / Magnets
	Torrent       string   `json:"torrent,omitempty"`        // Base64 encoded .torrent file
	ActionName    string   `json:"action,omitempty"`         // Preconfigured action name (empty for default)
	DownloadLimit string   `json:"download_limit,omitempty"` // e.g. "5M", "500K"
	UploadLimit   string   `json:"upload_limit,omitempty"`   // e.g. "1M"
	NotifyURL     string   `json:"notify_url,omitempty"`     // Custom Webhook notification URL
}

// TaskResponse is returned when a task is submitted
type TaskResponse struct {
	TaskIDs []string `json:"task_ids"`
	Status  string   `json:"status"`
	Message string   `json:"message"`
}
