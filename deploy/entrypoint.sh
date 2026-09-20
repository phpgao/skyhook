#!/bin/bash
set -e

# Default directories
DOWNLOAD_DIR="${DOWNLOAD_DIR:-/data/downloads}"
CONFIG_DIR="${CONFIG_DIR:-/data/config}"
mkdir -p "${DOWNLOAD_DIR}" "${CONFIG_DIR}" /etc/skyhook /root/.diskcli

# Ensure SkyHook configuration exists
CONFIG_FILE="${SKYHOOK_CONFIG:-/data/config/config.yaml}"
if [ ! -f "${CONFIG_FILE}" ]; then
    if [ -f /etc/skyhook/config.yaml ]; then
        CONFIG_FILE="/etc/skyhook/config.yaml"
    else
        echo "[SkyHook Init] No config found, creating default config at ${CONFIG_FILE}..."
        cp /etc/skyhook/config.default.yaml "${CONFIG_FILE}"
    fi
fi

# Override auth token from environment variable if provided
if [ -n "${AUTH_TOKEN}" ]; then
    echo "[SkyHook Init] Applying AUTH_TOKEN from environment variable..."
    sed -i "s/auth_token: .*/auth_token: \"${AUTH_TOKEN}\"/" "${CONFIG_FILE}"
fi

# If QUARK_COOKIE is supplied via environment variable, write to diskcli config
if [ -n "${QUARK_COOKIE}" ]; then
    echo "[SkyHook Init] Initializing diskcli configuration with provided QUARK_COOKIE..."
    mkdir -p /root/.diskcli
    cat << EOF > /root/.diskcli/config
[provider.quark]
cookie = "${QUARK_COOKIE}"
EOF
fi

# Start internal Aria2 RPC daemon if enabled (default true)
if [ "${ENABLE_ARIA2_RPC:-true}" = "true" ]; then
    ARIA2_SECRET="${ARIA2_SECRET:-skyhook-rpc-token}"
    echo "[SkyHook Init] Starting internal Aria2 RPC daemon on port 6800..."
    aria2c --enable-rpc \
           --rpc-listen-all=true \
           --rpc-listen-port=6800 \
           --rpc-secret="${ARIA2_SECRET}" \
           --rpc-allow-origin-all=true \
           --dir="${DOWNLOAD_DIR}" \
           --max-connection-per-server=16 \
           --split=16 \
           --min-split-size=1M \
           --continue=true \
           --daemon=true || echo "[SkyHook Init] Warning: aria2c daemon failed to start"
fi

# If command starts with an existing executable, run it directly
if command -v "$1" >/dev/null 2>&1 && [ "$1" != "skyhookd" ]; then
    exec "$@"
fi

if [ "$1" = "skyhookd" ]; then
    shift
fi

echo "[SkyHook Init] Starting SkyHook daemon with config: ${CONFIG_FILE}"
exec /usr/local/bin/skyhookd -c "${CONFIG_FILE}" "$@"
