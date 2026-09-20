package client

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Credentials represents configuration stored in .skyhook file
type Credentials struct {
	Server        string `yaml:"server"`
	Token         string `yaml:"token"`
	DefaultAction string `yaml:"default_action,omitempty"`
}

const CredentialsFileName = ".skyhook"

// FindCredentialsPath looks for .skyhook in current directory, then home directory
func FindCredentialsPath() string {
	// 1. Current working directory
	if _, err := os.Stat(CredentialsFileName); err == nil {
		return CredentialsFileName
	}

	// 2. User home directory
	if home, err := os.UserHomeDir(); err == nil {
		homePath := filepath.Join(home, CredentialsFileName)
		if _, err := os.Stat(homePath); err == nil {
			return homePath
		}
	}

	return ""
}

// DefaultCredentialsPath returns ~/.skyhook
func DefaultCredentialsPath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, CredentialsFileName)
	}
	return CredentialsFileName
}

// LoadCredentials loads credentials from .skyhook file if it exists
func LoadCredentials() (*Credentials, error) {
	path := FindCredentialsPath()
	if path == "" {
		return nil, nil // No file found, not an error
	}
	return LoadCredentialsFromPath(path)
}

// LoadCredentialsFromPath loads credentials from a specific file path
func LoadCredentialsFromPath(path string) (*Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file %s: %w", path, err)
	}

	var creds Credentials
	if err := yaml.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials file %s: %w", path, err)
	}

	return &creds, nil
}

// SaveCredentials writes credentials to a .skyhook file with 0600 permissions
func SaveCredentials(path string, creds Credentials) error {
	if path == "" {
		path = DefaultCredentialsPath()
	}

	data, err := yaml.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Restrict permissions to owner-only read/write (0600) for security
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write credentials to %s: %w", path, err)
	}

	return nil
}
