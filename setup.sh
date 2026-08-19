#!/usr/bin/env bash
#
# setup.sh — install qo on Linux (multi-distro)
#
# Builds the static qo binary and installs it to /usr/local/bin, along with the
# qo-check / qo-setup / qo-reset aliases the sandbox uses.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Open-Source-Community/qo2/ctf-improvements/setup.sh | bash
#
# Or from a checkout:
#   ./setup.sh
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

detect_pkg_mgr() {
    if have apt-get; then echo apt
    elif have dnf; then echo dnf
    elif have pacman; then echo pacman
    elif have zypper; then echo zypper
    elif have apk; then echo apk
    elif have microdnf; then echo microdnf
    else echo unknown; fi
}

install_deps() {
    [ "${QO_SKIP_DEPS:-0}" = "1" ] && { log "skipping package install (QO_SKIP_DEPS=1)"; return; }

    local mgr
    mgr=$(detect_pkg_mgr)
    log "detected package manager: $mgr"

    case "$mgr" in
        apt)
            sudo apt-get update -y
            sudo apt-get install -y git golang-go gcc libc6-dev tar gzip curl ca-certificates
            ;;
        dnf|microdnf)
            if command -v sudo >/dev/null 2>&1; then
                sudo dnf install -y git golang gcc glibc-devel tar gzip curl ca-certificates
            else
                dnf install -y git golang gcc glibc-devel tar gzip curl ca-certificates
            fi
            ;;
        pacman)
            sudo pacman -Sy --noconfirm --needed git go gcc tar gzip curl ca-certificates
            ;;
        zypper)
            sudo zypper install -y git go gcc glibc-devel tar gzip curl ca-certificates
            ;;
        apk)
            apk add --no-cache git go gcc musl-dev tar gzip curl ca-certificates
            ;;
        *)
            warn "unsupported package manager; assuming go is already installed"
            ;;
    esac

    if ! need go; then
        warn "go is not on PATH; install it from https://go.dev/dl/ or re-run with QO_SKIP_DEPS=0"
    fi
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

    log "installed: $dest"
}

if [ "$1" = "uninstall" ]; then
    for a in qo qo-check qo-setup qo-reset; do
        sudo rm -f "$INSTALL_DIR/$a"
        log "removed $INSTALL_DIR/$a"
    done
    exit 0
fi

# Build from a local checkout if present, otherwise clone a fresh copy.
if [ -f ./main.go ] && [ -d ./pkg ]; then
    log "building from local checkout: $PWD"
    install_deps
    build_and_install "$PWD"
else
    install_deps
    TMP_DIR=$(mktemp -d)
    trap 'rm -rf "$TMP_DIR"' EXIT
    log "cloning $REPO_URL ..."
    git clone --depth=1 "$REPO_URL" "$TMP_DIR/qo2"
    build_and_install "$TMP_DIR/qo2"
fi

log "done. Verify with: qo --help"