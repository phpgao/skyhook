package downloader

import (
	"log"
	"strings"

	"github.com/phpgao/skyhook/internal/config"
)

// BuildRegistryFromConfig instantiates downloaders based on config.yaml
func BuildRegistryFromConfig(cfg *config.Config) *Registry {
	var customDomains []string
	for _, d := range cfg.Downloaders {
		customDomains = append(customDomains, d.Domains...)
	}

	reg := NewRegistry(customDomains)

	if len(cfg.Downloaders) > 0 {
		for _, dc := range cfg.Downloaders {
			if dc.Enabled != nil && !*dc.Enabled {
				continue
			}

			var targetTypes []TargetType
			for _, tt := range dc.TargetTypes {
				targetTypes = append(targetTypes, TargetType(strings.ToLower(tt)))
			}
			if len(targetTypes) == 0 {
				targetTypes = []TargetType{TargetHTTP}
			}

			switch strings.ToLower(dc.Type) {
			case "aria2_rpc":
				rpcURL := dc.RPCURL
				if rpcURL == "" {
					rpcURL = cfg.Aria2.RPCURL
				}
				rpcSecret := dc.RPCSecret
				if rpcSecret == "" {
					rpcSecret = cfg.Aria2.RPCSecret
				}
				client := NewAria2Client(rpcURL, rpcSecret, nil)
				if dc.Priority > 0 {
					client.SetPriority(dc.Priority)
				}
				reg.Register(client)

			case "cli":
				p := dc.Priority
				if p <= 0 {
					p = 10
				}
				cliDriver := NewGenericCLIDownloader(
					dc.Name,
					dc.Bin,
					p,
					targetTypes,
					dc.Args,
					cfg.Aria2.DownloadDir,
				)
				reg.Register(cliDriver)

			case "native":
				nat := NewNativeHTTPDownloader(cfg.Aria2.DownloadDir, nil)
				reg.Register(nat)

			default:
				// Default to generic CLI driver
				p := dc.Priority
				if p <= 0 {
					p = 10
				}
				cliDriver := NewGenericCLIDownloader(
					dc.Name,
					dc.Bin,
					p,
					targetTypes,
					dc.Args,
					cfg.Aria2.DownloadDir,
				)
				reg.Register(cliDriver)
			}
		}
	} else {
		// Default suite when user configures no downloaders
		log.Printf("[Registry] No downloaders section in config; initializing standard multi-driver suite")

		// 1. Aria2 RPC (priority 88 for BT / Magnet)
		aria2Client := NewAria2Client(cfg.Aria2.RPCURL, cfg.Aria2.RPCSecret, nil)
		aria2Client.SetPriority(88)
		reg.Register(aria2Client)

		// 2. Aria2c CLI fork (priority 88 for HTTP)
		reg.Register(NewGenericCLIDownloader("aria2c", "", 88, []TargetType{TargetHTTP}, nil, cfg.Aria2.DownloadDir))

		// 3. Wget (priority 19 for HTTP)
		reg.Register(NewGenericCLIDownloader("wget", "", 19, []TargetType{TargetHTTP}, nil, cfg.Aria2.DownloadDir))

		// 4. Curl (priority 10 for HTTP)
		reg.Register(NewGenericCLIDownloader("curl", "", 10, []TargetType{TargetHTTP}, nil, cfg.Aria2.DownloadDir))

		// 5. Yt-Dlp (priority 10 for Video)
		reg.Register(NewGenericCLIDownloader("yt-dlp", "", 10, []TargetType{TargetVideo}, nil, cfg.Aria2.DownloadDir))
	}

	// Always guarantee pure Go Native Downloader is registered as ultimate fallback (priority 1)
	hasNative := false
	for _, d := range reg.All() {
		if d.Name() == "native" {
			hasNative = true
			break
		}
	}
	if !hasNative {
		reg.Register(NewNativeHTTPDownloader(cfg.Aria2.DownloadDir, nil))
	}

	return reg
}
