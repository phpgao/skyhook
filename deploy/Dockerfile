# ==============================================================================
# Stage 1: Build static binaries (SkyHook & diskcli)
# ==============================================================================
FROM --platform=$BUILDPLATFORM golang:alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Download SkyHook dependencies first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build statically linked SkyHook binaries
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /build/skyhookd ./cmd/skyhookd && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /build/skyhook ./cmd/skyhook

# Build diskcli (Quark / Multi-cloud disk CLI by @phpgao)
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go install -trimpath -ldflags="-s -w" github.com/phpgao/diskcli/cmd/diskcli@latest && \
    cp $(find /go/bin -type f -name diskcli | head -n 1) /build/diskcli

# ==============================================================================
# Stage 2: Minimal Alpine runtime with full download & cloud upload capabilities
# ==============================================================================
FROM alpine:latest

LABEL maintainer="SkyHook Team"
LABEL description="SkyHook - High performance remote relay, download pipeline engine with built-in diskcli (Quark pan)"

# Install all essential download engines and automation tools:
# - aria2: Multi-connection HTTP/FTP & BitTorrent/Metalink client + JSON-RPC daemon
# - curl & wget: Standard resilient CLI download drivers
# - yt-dlp & ffmpeg: Video/audio stream extraction and muxing from 1000+ sites
# - bash & jq: Shell script execution and JSON processing in pipeline hooks
# - ca-certificates & tzdata: Secure TLS connections and timezones
RUN apk update && apk add --no-cache \
    ca-certificates \
    tzdata \
    bash \
    curl \
    wget \
    aria2 \
    yt-dlp \
    ffmpeg \
    jq \
    && rm -rf /var/cache/apk/*

# Create application paths
RUN mkdir -p /etc/skyhook /data/downloads /data/config /root/.diskcli

# Copy compiled binaries from builder
COPY --from=builder /build/skyhookd /usr/local/bin/skyhookd
COPY --from=builder /build/skyhook /usr/local/bin/skyhook
COPY --from=builder /build/diskcli /usr/local/bin/diskcli

# Symlink quark-cli to diskcli for backward compatibility in action hooks
RUN ln -s /usr/local/bin/diskcli /usr/local/bin/quark-cli

# Copy default docker configuration and entrypoint
COPY deploy/config.docker.yaml /etc/skyhook/config.default.yaml
COPY deploy/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh /usr/local/bin/skyhookd /usr/local/bin/skyhook /usr/local/bin/diskcli /usr/local/bin/quark-cli

# Persistent volumes for downloads and configuration/state
VOLUME ["/data/downloads", "/data/config"]

# Expose SkyHook HTTP API (8080) and Aria2 JSON-RPC (6800)
EXPOSE 8080 6800

WORKDIR /data

ENTRYPOINT ["/entrypoint.sh"]
