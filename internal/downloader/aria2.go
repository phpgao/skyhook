package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Aria2Client implements Downloader for Aria2 JSON-RPC
type Aria2Client struct {
	rpcURL     string
	secret     string
	priority   int
	httpClient *http.Client
}

// NewAria2Client creates a new Aria2 Downloader
func NewAria2Client(rpcURL, secret string, client *http.Client) *Aria2Client {
	if client == nil {
		client = &http.Client{}
	}
	return &Aria2Client{
		rpcURL:     rpcURL,
		secret:     secret,
		priority:   88, // default priority 88 for Aria2 RPC
		httpClient: client,
	}
}

func (a *Aria2Client) Name() string {
	return "aria2_rpc"
}

func (a *Aria2Client) TargetTypes() []TargetType {
	return []TargetType{TargetBT, TargetMagnet, TargetHTTP}
}

func (a *Aria2Client) Priority() int {
	return a.priority
}

func (a *Aria2Client) SetPriority(p int) {
	a.priority = p
}

func (a *Aria2Client) IsAvailable(ctx context.Context) bool {
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := a.call(probeCtx, "aria2.getVersion", nil)
	return err == nil
}

type rpcRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      string        `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (a *Aria2Client) call(ctx context.Context, method string, params []interface{}) (json.RawMessage, error) {
	var fullParams []interface{}
	if a.secret != "" {
		fullParams = append(fullParams, "token:"+a.secret)
	}
	fullParams = append(fullParams, params...)

	reqBody := rpcRequest{
		JSONRPC: "2.0",
		ID:      "skyhook",
		Method:  method,
		Params:  fullParams,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal aria2 request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.rpcURL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute aria2 rpc call %s: %w", method, err)
	}
	defer httpResp.Body.Close()

	var resp rpcResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to decode aria2 response: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("aria2 error [%d]: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

func (a *Aria2Client) buildOptions(opts Options) map[string]interface{} {
	optMap := map[string]interface{}{
		"seed-time":                 "0",   // Default: stop seeding immediately on completion
		"seed-ratio":                "0.0",
		"split":                     "16",
		"max-connection-per-server": "16",
		"continue":                  "true", // 断点续传支持 (Resume partial downloads)
	}

	if opts.SeedTime != "" {
		optMap["seed-time"] = opts.SeedTime
	}
	if opts.SeedRatio != "" {
		optMap["seed-ratio"] = opts.SeedRatio
	}
	if opts.Dir != "" {
		optMap["dir"] = opts.Dir
	}
	if opts.DownloadLimit != "" && opts.DownloadLimit != "0" {
		optMap["max-download-limit"] = opts.DownloadLimit
	}
	if opts.UploadLimit != "" && opts.UploadLimit != "0" {
		optMap["max-upload-limit"] = opts.UploadLimit
	}
	if opts.Trackers != "" {
		optMap["bt-tracker"] = opts.Trackers
	}

	return optMap
}

func (a *Aria2Client) AddURI(ctx context.Context, uris []string, opts Options) (string, error) {
	options := a.buildOptions(opts)
	params := []interface{}{uris, options}

	raw, err := a.call(ctx, "aria2.addUri", params)
	if err != nil {
		return "", err
	}

	var gid string
	if err := json.Unmarshal(raw, &gid); err != nil {
		return "", fmt.Errorf("failed to parse GID from result: %w", err)
	}
	return gid, nil
}

func (a *Aria2Client) AddTorrent(ctx context.Context, base64Torrent string, opts Options) (string, error) {
	options := a.buildOptions(opts)
	params := []interface{}{base64Torrent, []string{}, options}

	raw, err := a.call(ctx, "aria2.addTorrent", params)
	if err != nil {
		return "", err
	}

	var gid string
	if err := json.Unmarshal(raw, &gid); err != nil {
		return "", fmt.Errorf("failed to parse GID from result: %w", err)
	}
	return gid, nil
}

type aria2RawStatus struct {
	GID             string `json:"gid"`
	Status          string `json:"status"`
	TotalLength     string `json:"totalLength"`
	CompletedLength string `json:"completedLength"`
	UploadLength    string `json:"uploadLength"`
	DownloadSpeed   string `json:"downloadSpeed"`
	UploadSpeed     string `json:"uploadSpeed"`
	ErrorMessage    string `json:"errorMessage"`
	Dir             string `json:"dir"`
	Files           []struct {
		Path            string `json:"path"`
		Length          string `json:"length"`
		CompletedLength string `json:"completedLength"`
	} `json:"files"`
	Bittorrent *struct {
		Info *struct {
			Name string `json:"name"`
		} `json:"info"`
	} `json:"bittorrent"`
}

func (a *Aria2Client) TellStatus(ctx context.Context, gid string) (*DownloadStatus, error) {
	raw, err := a.call(ctx, "aria2.tellStatus", []interface{}{gid})
	if err != nil {
		return nil, err
	}

	var rawStatus aria2RawStatus
	if err := json.Unmarshal(raw, &rawStatus); err != nil {
		return nil, fmt.Errorf("failed to parse status payload: %w", err)
	}

	status := &DownloadStatus{
		GID:          rawStatus.GID,
		Status:       rawStatus.Status,
		ErrorMessage: rawStatus.ErrorMessage,
		Dir:          rawStatus.Dir,
	}

	status.TotalLength, _ = strconv.ParseInt(rawStatus.TotalLength, 10, 64)
	status.CompletedLength, _ = strconv.ParseInt(rawStatus.CompletedLength, 10, 64)
	status.UploadLength, _ = strconv.ParseInt(rawStatus.UploadLength, 10, 64)
	status.DownloadSpeed, _ = strconv.ParseInt(rawStatus.DownloadSpeed, 10, 64)
	status.UploadSpeed, _ = strconv.ParseInt(rawStatus.UploadSpeed, 10, 64)

	if rawStatus.Bittorrent != nil && rawStatus.Bittorrent.Info != nil {
		status.TorrentName = rawStatus.Bittorrent.Info.Name
	}

	for _, f := range rawStatus.Files {
		l, _ := strconv.ParseInt(f.Length, 10, 64)
		c, _ := strconv.ParseInt(f.CompletedLength, 10, 64)
		status.Files = append(status.Files, FileInfo{
			Path:      f.Path,
			Length:    l,
			Completed: c,
		})
	}

	return status, nil
}

func (a *Aria2Client) ForceRemove(ctx context.Context, gid string) error {
	_, err := a.call(ctx, "aria2.forceRemove", []interface{}{gid})
	return err
}

func (a *Aria2Client) ResolveTargetPath(status *DownloadStatus) string {
	if status == nil {
		return ""
	}

	// Case 1: Multi-file torrent or single torrent directory
	if status.TorrentName != "" {
		potentialDir := filepath.Join(status.Dir, status.TorrentName)
		if fi, err := os.Stat(potentialDir); err == nil && fi.IsDir() {
			return potentialDir
		}
	}

	// Case 2: Inspect first file path
	if len(status.Files) > 0 && status.Files[0].Path != "" {
		firstPath := status.Files[0].Path
		if len(status.Files) > 1 {
			// If multi-file, return the common parent directory
			if status.TorrentName != "" {
				return filepath.Join(status.Dir, status.TorrentName)
			}
		}
		return firstPath
	}

	return ""
}
