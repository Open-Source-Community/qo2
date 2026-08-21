#!/usr/bin/env bash
#
# setup.sh — install qo on Linux (multi-distro)
#
# Binary-first: downloads a prebuilt static binary from GitHub Releases.
# Only falls back to building from source if Go ≥ 1.26 is already installed.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Open-Source-Community/qo2/main/setup.sh | bash
#
# Or from a checkout:
#   ./setup.sh
#
# Distribute the static binary directly (e.g. alongside test.enc):
#   ./setup.sh ./qo        # path to the downloaded qo binary
#
set -e

APP_NAME="qo"
INSTALL_DIR="/usr/local/bin"
REPO_URL="https://github.com/Open-Source-Community/qo2"
RELEASE_URL="https://github.com/Open-Source-Community/qo2/releases/latest/download"

log()  { printf '\033[1;36m[qo]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[qo]\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m[qo]\033[0m %s\n' "$*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1; }

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64) echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *) echo "" ;;
    esac
}

install_binary() {
    local src="$1"

    if [ ! -f "$src" ]; then
        die "binary not found: $src"
    fi
    if [ ! -x "$src" ]; then
        chmod +x "$src"
    fi

    local dest="$INSTALL_DIR/$APP_NAME"
    if [ -w "$INSTALL_DIR" ]; then
        mv -f "$src" "$dest"
    else
        sudo mv -f "$src" "$dest"
    fi
    sudo chmod 755 "$dest"

    for alias in qo-check qo-setup qo-reset; do
        sudo ln -sf "$dest" "$INSTALL_DIR/$alias"
    done

    # Symlink into /usr/bin so sudo can find qo on distros where
    # /usr/local/bin is not in sudo's secure_path (Fedora, CentOS).
    if [ -d /usr/bin ]; then
        sudo ln -sf "$dest" /usr/bin/$APP_NAME
        for alias in qo-check qo-setup qo-reset; do
            sudo ln -sf "$dest" /usr/bin/$alias
        done
    fi

    log "installed: $dest"
    log "done. Verify with: qo --help"
}

download_binary() {
    local arch
    arch=$(detect_arch)
    if [ -z "$arch" ]; then
        warn "unsupported architecture $(uname -m); cannot download prebuilt binary"
        return 1
    fi

    local url="$RELEASE_URL/${APP_NAME}_linux_${arch}"
    local tmp
    tmp=$(mktemp)
    log "downloading prebuilt binary: $url"
    if curl -fsSL --max-time 60 "$url" -o "$tmp"; then
        install_binary "$tmp"
        return 0
    else
        rm -f "$tmp"
        warn "download failed (no internet or release not found)"
        return 1
    fi
}

# Build from source only if Go is already installed.
build_from_source() {
    if ! need go; then
        return 1
    fi

    local go_minor
    go_minor=$(go version 2>/dev/null | sed -n 's/.*go\([0-9][0-9]*\.[0-9][0-9]*\).*/\1/p')
    if [ -z "$go_minor" ]; then
        return 1
    fi

    local min="1.26"
    if [ "$(printf '%s\n%s\n' "$min" "$go_minor" | sort -V | head -1)" != "$min" ]; then
        warn "go $go_minor found but >= $min required; cannot build from source"
        return 1
    fi

    log "go $go_minor detected; building from source..."
    TMP_DIR=$(mktemp -d)
    trap 'rm -rf "$TMP_DIR"' EXIT
    git clone --depth=1 "$REPO_URL" "$TMP_DIR/qo2" 2>/dev/null || return 1
    cd "$TMP_DIR/qo2"
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$APP_NAME" .
    install_binary "./$APP_NAME"
}

# --- main ---

# Uninstall
if [ "$1" = "uninstall" ]; then
    for a in qo qo-check qo-setup qo-reset; do
        sudo rm -f "$INSTALL_DIR/$a"
        sudo rm -f "/usr/bin/$a"
        log "removed $INSTALL_DIR/$a"
    done
    exit 0
fi

# Install from local binary if provided
if [ -n "$1" ] && [ -f "$1" ]; then
    install_binary "$1"
    exit 0
elif [ -f "./qo" ] && [ -x "./qo" ] && [ "$1" != "uninstall" ]; then
    install_binary "./qo"
    exit 0
fi

# Try downloading prebuilt binary (fast path — no Go needed)
if download_binary; then
    exit 0
fi

# Fallback: build from source if Go is available
if build_from_source; then
    exit 0
fi

# Nothing worked
die "install failed. Options:
  1. Install Go >= 1.26 from https://go.dev/dl/ and re-run this script
  2. Download the prebuilt binary from $REPO_URL/releases and run: ./setup.sh ./qo"
