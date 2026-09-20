package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/phpgao/skyhook/internal/model"
	"github.com/phpgao/skyhook/pkg/client"
)

func TestClient_SubmitSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check token
		auth := r.Header.Get("Authorization")
		if auth != "Bearer valid-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var req model.TaskRequest
		json.NewDecoder(r.Body).Decode(&req)

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(model.TaskResponse{
			TaskIDs: []string{"1001", "1002"},
			Status:  "queued",
			Message: "queued",
		})
	}))
	defer ts.Close()

	cli := client.NewClient(ts.URL, "valid-token")
	resp, err := cli.Submit(context.Background(), model.TaskRequest{
		URLs: []string{"https://example.com/1.zip", "https://example.com/2.zip"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.TaskIDs) != 2 {
		t.Errorf("expected 2 task IDs, got %d", len(resp.TaskIDs))
	}
}

func TestClient_Unauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	cli := client.NewClient(ts.URL, "bad-token")
	_, err := cli.Submit(context.Background(), model.TaskRequest{
		URLs: []string{"https://example.com/1.zip"},
	})
	if err == nil {
		t.Fatalf("expected authorization error")
	}
}
