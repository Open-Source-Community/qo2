#!/usr/bin/env bash
#
# setup.sh — install qo on Linux (multi-distro)
#
# Builds the static qo binary and installs it to /usr/local/bin, along with the
# qo-check / qo-setup / qo-reset aliases the sandbox uses.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Open-Source-Community/qo2/main/setup.sh | bash
#
# Or from a checkout:
#   ./setup.sh
#
# No network access to the repo? Distribute the static binary directly
# (e.g. alongside test.enc) and install it with:
#   ./setup.sh ./qo        # path to the downloaded qo binary
#   ./setup.sh qo          # or any local file
#
# Set QO_SKIP_DEPS=1 to skip the package-manager step (e.g. you already have go).
set -e

APP_NAME="qo"
INSTALL_DIR="/usr/local/bin"
REPO_URL="https://github.com/Open-Source-Community/qo2.git"

log()  { printf '\033[1;36m[qo]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[qo]\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m[qo]\033[0m %s\n' "$*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1; }

have() { command -v "$1" >/dev/null 2>&1; }

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

    install_sudo_symlinks "$dest"

    log "installed: $dest"
    log "done. Verify with: qo --help"
}

# install_sudo_symlinks also links into /usr/bin. Fedora's sudo default
# secure_path is /sbin:/bin:/usr/sbin:/usr/bin — it excludes /usr/local/bin,
# so `sudo qo` would report "command not found" even though qo is installed.
# /usr/bin is in the sudo PATH on every distro, so the symlink makes
# `sudo qo start ...` work everywhere.
install_sudo_symlinks() {
    local dest="$1"
    if [ ! -d /usr/bin ]; then
        return
    fi
    sudo ln -sf "$dest" /usr/bin/$APP_NAME
    for alias in qo-check qo-setup qo-reset; do
        sudo ln -sf "$dest" /usr/bin/$alias
    done
}

detect_pkg_mgr() {
    if have apt-get; then echo apt
    elif have dnf; then echo dnf
    elif have pacman; then echo pacman
    elif have zypper; then echo zypper
    elif have apk; then echo apk
    elif have microdnf; then echo microdnf
    else echo unknown; fi
}

MIN_GO="1.26"   # go.mod requirement; older toolchains cannot build the module
GO_URL_BASE="https://go.dev/dl"

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64) echo amd64 ;;
        aarch64|arm64) echo arm64 ;;
        *) echo "" ;;
    esac
}

# Fetch the latest stable Go version from go.dev (e.g. "1.26.6").
fetch_latest_go() {
    local v
    v=$(curl -fsSL --max-time 15 "https://go.dev/VERSION?m=text" 2>/dev/null | head -1 | tr -d ' \n')
    case "$v" in
        go[0-9]*.[0-9]*.[0-9]*) echo "${v#go}" ;;
        *) echo "" ;;
    esac
}

go_major_minor() {
    go version 2>/dev/null | sed -n 's/.*go\([0-9][0-9]*\.[0-9][0-9]*\).*/\1/p'
}

# true if the `go` on PATH is new enough to build the module.
have_go_new_enough() {
    need go || return 1
    local v
    v=$(go_major_minor)
    [ -n "$v" ] && [ "$(printf '%s\n%s\n' "$MIN_GO" "$v" | sort -V | head -1)" = "$MIN_GO" ]
}

# Ensure a Go toolchain new enough to build (distro packages are often ancient:
# Ubuntu 22.04 ships 1.18, Debian bookworm 1.19, Fedora 39 1.21 — all fail on a
# `go 1.26` module). Fall back to the latest official tarball, like CI does.
ensure_go() {
    if have_go_new_enough; then
        log "go $MIN_GO+ detected: $(go version)"
        return
    fi

    local arch
    arch=$(detect_arch)
    if [ -z "$arch" ]; then
        die "unsupported architecture $(uname -m); install Go >= $MIN_GO manually"
    fi

    local ver
    ver=$(fetch_latest_go)
    if [ -z "$ver" ]; then
        warn "could not query go.dev for the latest version; falling back to $MIN_GO.0"
        ver="$MIN_GO.0"
    fi

    log "distro Go too old or missing; installing Go $ver (linux/$arch) from go.dev ..."
    local tmp
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT
    curl -fsSL "$GO_URL_BASE/go$ver.linux-$arch.tar.gz" -o "$tmp/go.tgz"
    if [ -d /usr/local/go ]; then
        sudo rm -rf /usr/local/go
    fi
    sudo tar -C /usr/local -xzf "$tmp/go.tgz"
    rm -rf "$tmp"
    trap - EXIT

    export PATH=/usr/local/go/bin:$PATH
    need go || die "failed to install Go"
    log "installed: $(go version)"
}

install_deps() {
    [ "${QO_SKIP_DEPS:-0}" = "1" ] && { log "skipping package install (QO_SKIP_DEPS=1)"; return; }

    local mgr
    mgr=$(detect_pkg_mgr)
    log "detected package manager: $mgr"

    case "$mgr" in
        apt)
            sudo apt-get update -y
            sudo apt-get install -y git tar gzip curl ca-certificates
            ;;
        dnf|microdnf)
            if command -v sudo >/dev/null 2>&1; then
                sudo dnf install -y git tar gzip curl ca-certificates
            else
                dnf install -y git tar gzip curl ca-certificates
            fi
            ;;
        pacman)
            sudo pacman -Sy --noconfirm --needed git tar gzip curl ca-certificates
            ;;
        zypper)
            sudo zypper install -y git tar gzip curl ca-certificates
            ;;
        apk)
            apk add --no-cache git tar gzip curl ca-certificates
            ;;
        *)
            warn "unsupported package manager; assuming git/tar/curl are already installed"
            ;;
    esac
}

build_and_install() {
    local srcdir="$1"

    cd "$srcdir"

    # Static build: the sandbox copies this binary into the chroot as /bin/qo-check,
    # so it must not depend on host glibc.
    log "building static binary (CGO_ENABLED=0)..."
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$APP_NAME" .

    local dest="$INSTALL_DIR/$APP_NAME"
    if [ -w "$INSTALL_DIR" ]; then
        mv -f "$APP_NAME" "$dest"
    else
        sudo mv -f "$APP_NAME" "$dest"
    fi
    sudo chmod 755 "$dest"

    # Aliases the sandbox expects; they share the same binary and dispatch on argv[0].
    for alias in qo-check qo-setup qo-reset; do
        sudo ln -sf "$dest" "$INSTALL_DIR/$alias"
    done

    install_sudo_symlinks "$dest"

    log "installed: $dest"
}

if [ "$1" = "uninstall" ]; then
    for a in qo qo-check qo-setup qo-reset; do
        sudo rm -f "$INSTALL_DIR/$a"
        log "removed $INSTALL_DIR/$a"
    done
    exit 0
fi

# Prebuilt binary path: if a local qo binary is given (or present in cwd),
# install it directly without cloning or building.
if [ -n "$1" ] && [ -f "$1" ]; then
    install_binary "$1"
    exit 0
elif [ -f "./qo" ] && [ -x "./qo" ] && [ "$1" != "uninstall" ]; then
    install_binary "./qo"
    exit 0
fi

# Build from a local checkout if present, otherwise clone a fresh copy.
if [ -f ./main.go ] && [ -d ./pkg ]; then
    log "building from local checkout: $PWD"
    install_deps
    ensure_go
    build_and_install "$PWD"
else
    install_deps
    ensure_go
    TMP_DIR=$(mktemp -d)
    trap 'rm -rf "$TMP_DIR"' EXIT
    log "cloning $REPO_URL ..."
    git clone --depth=1 "$REPO_URL" "$TMP_DIR/qo2" || die "cannot clone $REPO_URL — check your network/git access, or download the qo binary and run: ./setup.sh ./qo"
    build_and_install "$TMP_DIR/qo2"
fi

log "done. Verify with: qo --help"