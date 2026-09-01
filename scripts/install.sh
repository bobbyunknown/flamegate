#!/usr/bin/env bash
# FlameGate installer for macOS and Linux
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/bobbyunknown/flamegate/main/scripts/install.sh | bash

set -euo pipefail

GITHUB_REPO="bobbyunknown/flamegate"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
BIN_NAME="flamegate"

# Colors
BOLD="$(tput bold 2>/dev/null || echo '')"
GREEN="$(tput setaf 2 2>/dev/null || echo '')"
YELLOW="$(tput setaf 3 2>/dev/null || echo '')"
BLUE="$(tput setaf 4 2>/dev/null || echo '')"
CYAN="$(tput setaf 6 2>/dev/null || echo '')"
RESET="$(tput sgr0 2>/dev/null || echo '')"

info() {
    printf "${CYAN}==>${RESET} ${BOLD}%s${RESET}\n" "$1"
}

success() {
    printf "${GREEN}✓${RESET} ${BOLD}%s${RESET}\n" "$1"
}

warn() {
    printf "${YELLOW}!${RESET} %s\n" "$1"
}

error() {
    printf "${BOLD}error:${RESET} %s\n" "$1" >&2
    exit 1
}

# 1. Detect OS
detect_os() {
    local os
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    case "$os" in
        linux)  echo "linux" ;;
        darwin) echo "darwin" ;;
        *)      error "Unsupported operating system: $os (only Linux and macOS are supported)" ;;
    esac
}

# 2. Detect Architecture
detect_arch() {
    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64) echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        *)             error "Unsupported CPU architecture: $arch (only amd64 and arm64 are supported)" ;;
    esac
}

# 3. Main Installation Routine
main() {
    local os arch target_asset download_url temp_file
    os="$(detect_os)"
    arch="$(detect_arch)"
    target_asset="${BIN_NAME}-${os}-${arch}"

    printf "\n${BOLD}${BLUE}FlameGate Installer${RESET}\n"
    printf "Detected platform: ${BOLD}%s (%s)${RESET}\n\n" "$os" "$arch"

    info "Fetching latest FlameGate release..."

    # Determine downloader (curl or wget)
    if command -v curl >/dev/null 2>&1; then
        download_cmd="curl -fsSL"
    elif command -v wget >/dev/null 2>&1; then
        download_cmd="wget -qO-"
    else
        error "Neither 'curl' nor 'wget' was found on your system. Please install one of them."
    fi

    # Try latest release first, fallback to nightly
    download_url="https://github.com/${GITHUB_REPO}/releases/latest/download/${target_asset}"
    temp_file="$(mktemp -t flamegate-install.XXXXXX)"
    trap 'rm -f "$temp_file"' EXIT

    info "Downloading binary from GitHub Releases..."
    if ! curl -fsSL -L -o "$temp_file" "$download_url" 2>/dev/null; then
        warn "Latest release not found, falling back to nightly release..."
        download_url="https://github.com/${GITHUB_REPO}/releases/download/nightly/${target_asset}"
        if ! curl -fsSL -L -o "$temp_file" "$download_url"; then
            error "Failed to download $target_asset from $download_url"
        fi
    fi

    # Prepare installation directory
    mkdir -p "$INSTALL_DIR"
    chmod +x "$temp_file"

    local dest_path="${INSTALL_DIR}/${BIN_NAME}"
    mv -f "$temp_file" "$dest_path"
    chmod +x "$dest_path"

    success "FlameGate installed to: ${BOLD}${dest_path}${RESET}"

    # Verify binary execution
    if [ -x "$dest_path" ]; then
        local version_output
        version_output="$("$dest_path" version 2>/dev/null || echo 'installed')"
        printf "  Version: %s\n\n" "$version_output"
    fi

    # Check if INSTALL_DIR is in PATH
    case ":$PATH:" in
        *":$INSTALL_DIR:"*) ;;
        *)
            warn "${INSTALL_DIR} is not in your current PATH."
            printf "  To use '${BOLD}flamegate${RESET}' from anywhere, add this to your shell profile (~/.zshrc or ~/.bashrc):\n\n"
            printf "    ${CYAN}export PATH=\"%s:\$PATH\"${RESET}\n\n" "$INSTALL_DIR"
            ;;
    esac

    printf "${BOLD}Quick start:${RESET}\n"
    printf "  1. Bootstrap API key : ${CYAN}flamegate bootstrap${RESET}\n"
    printf "  2. Start gateway     : ${CYAN}flamegate${RESET}\n"
    printf "  3. Open Dashboard    : ${CYAN}http://localhost:20180${RESET}\n"
    printf "  4. LLM Proxy API     : ${CYAN}http://localhost:20181/v1${RESET}\n\n"
}

main "$@"
