package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/phpgao/skyhook/internal/model"
	"github.com/phpgao/skyhook/pkg/client"
)

func getEnvOrDefault(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

func main() {
	// 1. Try loading credentials from .skyhook file
	fileCreds, _ := client.LoadCredentials()

	defaultServer := "http://127.0.0.1:8080"
	defaultToken := ""
	defaultAction := ""

	if fileCreds != nil {
		if fileCreds.Server != "" {
			defaultServer = fileCreds.Server
		}
		if fileCreds.Token != "" {
			defaultToken = fileCreds.Token
		}
		if fileCreds.DefaultAction != "" {
			defaultAction = fileCreds.DefaultAction
		}
	}

	// Environment variable overrides
	defaultServer = getEnvOrDefault("SKYHOOK_SERVER", defaultServer)
	defaultToken = getEnvOrDefault("SKYHOOK_TOKEN", defaultToken)

	// Subcommand routing
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "login":
			handleLogin(os.Args[2:], defaultServer, defaultToken)
			return
		case "status", "list", "ps":
			handleStatus(os.Args[1], os.Args[2:], defaultServer, defaultToken)
			return
		}
	}

	fs := flag.NewFlagSet("skyhook", flag.ExitOnError)

	configFile := fs.String("c", "", "Path to custom .skyhook credentials file")
	serverURL := fs.String("s", "", "Explicit SkyHook server URL (overrides .skyhook / env)")
	token := fs.String("t", "", "Explicit authentication token (overrides .skyhook / env)")
	action := fs.String("a", defaultAction, "Preconfigured action name (empty for server default)")
	limitDL := fs.String("limit-dl", "", "Download speed limit (e.g. '10M', '500K')")
	limitUP := fs.String("limit-up", "", "Upload speed limit (e.g. '1M', '200K')")
	notifyURL := fs.String("notify", "", "Custom Webhook notification URL")

	fs.Usage = func() {
		fmt.Printf(`SkyHook (天钩) - Remote Relay & Automation CLI

Usage:
  skyhook [options] <url|magnet|torrent_file>...
  skyhook status [options] [task_gid]
  skyhook list [options]
  skyhook login -s <server> -t <token> [--local]

Commands:
  status [gid]
        Query download status of all tasks, or details for a specific task GID
  list / ps
        List all tracked download tasks
  login
        Save server address and token into .skyhook file

Options:
  -c string
        Path to custom .skyhook credentials file (default: ~/.skyhook or ./.skyhook)
  -s string
        SkyHook server URL (explicit override, current default: %s)
  -t string
        Authentication token (explicit override, current default: %s)
  -a string
        Action name configured on server (defaults to server's default_action)
  --limit-dl string
        Download rate limit (e.g. "10M", "500K")
  --limit-up string
        Upload rate limit (e.g. "1M", "200K")
  --notify string
        Custom Webhook notification URL

Precedence Order:
  Explicit CLI flags (-s, -t) > Environment variables > .skyhook file > Defaults

Examples:
  # 1. 使用 .skyhook 免密投送（推荐日常使用）
  skyhook https://example.com/1.zip https://example.com/2.tar.gz

  # 2. 显式直接指定服务器与 Token（完全覆盖 .skyhook 文件）
  skyhook -s http://1.2.3.4:8080 -t other-secret-token https://example.com/1.zip

  # 3. 指定自定义配置文件
  skyhook -c /path/to/my-vps.skyhook "magnet:?xt=urn:btih:..."

  # 4. 本地种子文件并限制 VPS 下载速度为 5M，做种速度为 500K
  skyhook -a quark --limit-dl 5M --limit-up 500K /path/to/ubuntu.torrent
`, defaultServer, maskToken(defaultToken))
	}

	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}

	targets := fs.Args()
	if len(targets) == 0 {
		fs.Usage()
		os.Exit(0)
	}

	// 1. If custom config file was passed, load it
	if *configFile != "" {
		customCreds, err := client.LoadCredentialsFromPath(*configFile)
		if err != nil {
			fmt.Printf("❌ Failed to load credentials from %s: %v\n", *configFile, err)
			os.Exit(1)
		}
		if customCreds.Server != "" {
			defaultServer = customCreds.Server
		}
		if customCreds.Token != "" {
			defaultToken = customCreds.Token
		}
		if customCreds.DefaultAction != "" && *action == "" {
			*action = customCreds.DefaultAction
		}
	}

	// 2. Explicit CLI flags have the highest priority
	finalServer := defaultServer
	if *serverURL != "" {
		finalServer = *serverURL
	}

	finalToken := defaultToken
	if *token != "" {
		finalToken = *token
	}

	cli := client.NewClient(finalServer, finalToken)

	// Quick health check
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := cli.Health(ctx); err != nil {
		fmt.Printf("⚠️  Warning: Cannot connect to SkyHook server at %s: %v\n", finalServer, err)
		fmt.Println("Proceeding anyway with submission...")
	}

	taskReq := model.TaskRequest{
		ActionName:    *action,
		DownloadLimit: *limitDL,
		UploadLimit:   *limitUP,
		NotifyURL:     *notifyURL,
	}

	for _, target := range targets {
		if strings.HasSuffix(strings.ToLower(target), ".torrent") {
			content, err := os.ReadFile(target)
			if err != nil {
				fmt.Printf("❌ Failed to read torrent file %q: %v\n", target, err)
				os.Exit(1)
			}
			taskReq.Torrent = base64.StdEncoding.EncodeToString(content)
			fmt.Printf("📦 Loaded torrent file: %s (%d bytes)\n", target, len(content))
		} else {
			taskReq.URLs = append(taskReq.URLs, target)
		}
	}

	submitCtx, submitCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer submitCancel()

	targetCount := len(taskReq.URLs)
	if taskReq.Torrent != "" {
		targetCount++
	}

	fmt.Printf("🚀 Submitting %d download target(s) to %s...\n", targetCount, *serverURL)
	resp, err := cli.Submit(submitCtx, taskReq)
	if err != nil {
		fmt.Printf("❌ Submission failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ %s\n", resp.Message)
	fmt.Printf("📋 Task GIDs:\n")
	for _, gid := range resp.TaskIDs {
		fmt.Printf("   • %s\n", gid)
	}
}

func handleLogin(args []string, defaultServer, defaultToken string) {
	loginFs := flag.NewFlagSet("login", flag.ExitOnError)
	srv := loginFs.String("s", defaultServer, "SkyHook Server URL")
	tok := loginFs.String("t", defaultToken, "Authentication Token")
	local := loginFs.Bool("local", false, "Save to ./.skyhook in current directory instead of ~/.skyhook")
	defAct := loginFs.String("a", "", "Default Action")

	if err := loginFs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *tok == "" {
		fmt.Println("❌ Error: token cannot be empty. Use: skyhook login -s <server> -t <token>")
		os.Exit(1)
	}

	targetPath := client.DefaultCredentialsPath()
	if *local {
		targetPath = client.CredentialsFileName
	}

	creds := client.Credentials{
		Server:        *srv,
		Token:         *tok,
		DefaultAction: *defAct,
	}

	if err := client.SaveCredentials(targetPath, creds); err != nil {
		fmt.Printf("❌ Failed to save credentials to %s: %v\n", targetPath, err)
		os.Exit(1)
	}

	fmt.Printf("✅ Authentication information successfully saved to %s (permissions 0600)\n", targetPath)
	fmt.Printf("   Server: %s\n", *srv)
	fmt.Printf("   Token:  %s\n", maskToken(*tok))
	fmt.Println("You can now run 'skyhook <url>' directly without passing server and token flags!")
}

func maskToken(tok string) string {
	if tok == "" {
		return "<none>"
	}
	if len(tok) <= 6 {
		return "******"
	}
	return tok[:3] + "..." + tok[len(tok)-3:]
}

func handleStatus(cmdName string, args []string, defaultServer, defaultToken string) {
	statusFs := flag.NewFlagSet(cmdName, flag.ExitOnError)
	configFile := statusFs.String("c", "", "Path to custom .skyhook credentials file")
	serverURL := statusFs.String("s", "", "SkyHook server URL (overrides .skyhook / env)")
	token := statusFs.String("t", "", "Authentication token (overrides .skyhook / env)")

	statusFs.Usage = func() {
		fmt.Printf(`Usage: skyhook %s [options] [task_gid]

Query download task status or list all tasks.

Options:
  -c string   Path to custom .skyhook credentials file
  -s string   SkyHook server URL
  -t string   Authentication token

Examples:
  skyhook status                 # List all tasks
  skyhook list                   # Alias for listing all tasks
  skyhook status <task_gid>      # Query details for a specific task
`, cmdName)
	}

	if err := statusFs.Parse(args); err != nil {
		os.Exit(1)
	}

	finalServer := defaultServer
	finalToken := defaultToken

	if *configFile != "" {
		customCreds, err := client.LoadCredentialsFromPath(*configFile)
		if err != nil {
			fmt.Printf("❌ Failed to load credentials from %s: %v\n", *configFile, err)
			os.Exit(1)
		}
		if customCreds.Server != "" {
			finalServer = customCreds.Server
		}
		if customCreds.Token != "" {
			finalToken = customCreds.Token
		}
	}

	if *serverURL != "" {
		finalServer = *serverURL
	}
	if *token != "" {
		finalToken = *token
	}

	cli := client.NewClient(finalServer, finalToken)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	remaining := statusFs.Args()
	if len(remaining) > 0 {
		// Query specific task
		gid := remaining[0]
		task, err := cli.GetTask(ctx, gid)
		if err != nil {
			fmt.Printf("❌ Failed to query task %q: %v\n", gid, err)
			os.Exit(1)
		}

		fmt.Printf("📋 Task Details [%s]:\n", task.GID)
		fmt.Printf("  Status:     %s\n", formatStatus(task.Status))
		if task.Action.Name != "" {
			fmt.Printf("  Action:     %s (clean_at_end: %v)\n", task.Action.Name, task.Action.CleanAtEnd)
		}
		fmt.Printf("  Created At: %s\n", task.CreatedAt.Local().Format("2006-01-02 15:04:05"))
		if !task.UpdatedAt.IsZero() {
			fmt.Printf("  Updated At: %s\n", task.UpdatedAt.Local().Format("2006-01-02 15:04:05"))
		}
		fmt.Printf("  Target URL: %s\n", task.URL)
		if task.ErrorMsg != "" {
			fmt.Printf("  Error Msg:  %s\n", task.ErrorMsg)
		}
		return
	}

	// List all tasks
	tasks, err := cli.ListTasks(ctx)
	if err != nil {
		fmt.Printf("❌ Failed to list tasks: %v\n", err)
		os.Exit(1)
	}

	if len(tasks) == 0 {
		fmt.Println("No download tasks found on server.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "TASK GID\tSTATUS\tACTION\tCREATED AT\tTARGET URL")
	for _, t := range tasks {
		actionName := t.Action.Name
		if actionName == "" {
			actionName = "-"
		}
		displayURL := t.URL
		if len(displayURL) > 60 {
			displayURL = displayURL[:57] + "..."
		}
		createdAt := t.CreatedAt.Local().Format("2006-01-02 15:04:05")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.GID, formatStatus(t.Status), actionName, createdAt, displayURL)
	}
	w.Flush()
}

func formatStatus(s model.TaskStatus) string {
	switch s {
	case model.StatusCompleted:
		return "🟢 completed"
	case model.StatusDownloading:
		return "🔵 downloading"
	case model.StatusProcessing:
		return "🟣 processing"
	case model.StatusFailed:
		return "🔴 failed"
	case model.StatusPending:
		return "⏳ pending"
	case model.StatusCanceled:
		return "⚪ canceled"
	default:
		return string(s)
	}
}
