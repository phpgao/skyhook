package client_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/phpgao/skyhook/pkg/client"
)

func TestCredentials_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, client.CredentialsFileName)

	origCreds := client.Credentials{
		Server:        "https://vps.example.com:8080",
		Token:         "secret-jwt-token-xyz",
		DefaultAction: "quark",
	}

	if err := client.SaveCredentials(credPath, origCreds); err != nil {
		t.Fatalf("failed to save credentials: %v", err)
	}

	// Verify file mode is 0600
	info, err := os.Stat(credPath)
	if err != nil {
		t.Fatalf("failed to stat credentials file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected permissions 0600, got %o", info.Mode().Perm())
	}

	// Read and verify
	data, err := os.ReadFile(credPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if len(data) == 0 {
		t.Fatalf("credentials file is empty")
	}
}
