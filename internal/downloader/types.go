package downloader

import (
	"net/url"
	"strings"
)

// TargetType classifies the category of download target
type TargetType string

const (
	TargetHTTP   TargetType = "http"
	TargetBT     TargetType = "bt"
	TargetMagnet TargetType = "magnet"
	TargetVideo  TargetType = "video"
)

var defaultVideoDomains = []string{
	"youtube.com",
	"youtu.be",
	"bilibili.com",
	"b23.tv",
	"twitter.com",
	"x.com",
	"vimeo.com",
	"tiktok.com",
	"douyin.com",
}

// DetectTargetType detects the target type from a URL or raw input
func DetectTargetType(input string, customVideoDomains []string) TargetType {
	lower := strings.ToLower(strings.TrimSpace(input))

	// 1. Magnet link
	if strings.HasPrefix(lower, "magnet:?") {
		return TargetMagnet
	}

	// 2. Torrent file
	if strings.HasSuffix(lower, ".torrent") {
		return TargetBT
	}

	// 3. Check for video domains
	domains := defaultVideoDomains
	if len(customVideoDomains) > 0 {
		domains = append(domains, customVideoDomains...)
	}

	parsed, err := url.Parse(input)
	if err == nil && parsed.Host != "" {
		host := strings.ToLower(parsed.Host)
		for _, domain := range domains {
			if strings.Contains(host, domain) {
				return TargetVideo
			}
		}
	}

	// 4. Default HTTP/HTTPS
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "ftp://") {
		return TargetHTTP
	}

	// Fallback to HTTP
	return TargetHTTP
}
