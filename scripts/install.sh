#!/bin/bash
set -euo pipefail

REPO="DanielJonesEB/detergent"
BINARY="detergent"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[info]${NC} $*"; }
warn()  { echo -e "${YELLOW}[warn]${NC} $*"; }
error() { echo -e "${RED}[error]${NC} $*" >&2; }
fatal() { error "$@"; exit 1; }

detect_platform() {
    local os arch

    case "$(uname -s)" in
        Darwin)  os="darwin" ;;
        Linux)   os="linux" ;;
        MINGW*|MSYS*|CYGWIN*) os="windows" ;;
        *) fatal "Unsupported operating system: $(uname -s)" ;;
    esac

    case "$(uname -m)" in
        x86_64|amd64)  arch="amd64" ;;
        arm64|aarch64) arch="arm64" ;;
        *) fatal "Unsupported architecture: $(uname -m)" ;;
    esac

    echo "${os}_${arch}"
}

get_latest_version() {
    local version
    version=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep '"tag_name"' \
        | head -1 \
        | sed 's/.*"tag_name": *"//;s/".*//')

    if [ -z "$version" ]; then
        return 1
    fi
    echo "$version"
}

install_from_release() {
    local platform="$1"
    local version

    info "Checking latest release..."
    version=$(get_latest_version) || return 1

    local os arch ext
    os=$(echo "$platform" | cut -d_ -f1)
    arch=$(echo "$platform" | cut -d_ -f2)

    if [ "$os" = "windows" ]; then
        ext="zip"
    else
        ext="tar.gz"
    fi

    local archive_name="${BINARY}_${version#v}_${os}_${arch}.${ext}"
    local url="https://github.com/${REPO}/releases/download/${version}/${archive_name}"

    info "Downloading ${BINARY} ${version} for ${platform}..."

    local tmpdir
    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT

    if ! curl -fsSL "$url" -o "${tmpdir}/${archive_name}"; then
        warn "Download failed for ${url}"
        return 1
    fi

    info "Extracting..."
    cd "$tmpdir"
    if [ "$ext" = "zip" ]; then
        unzip -q "$archive_name"
    else
        tar xzf "$archive_name"
    fi

    if [ ! -f "${BINARY}" ]; then
        warn "Binary not found in archive"
        return 1
    fi

    chmod +x "${BINARY}"

    # macOS code signing
    if [ "$os" = "darwin" ]; then
        info "Signing binary for macOS..."
        codesign --sign - --force "${BINARY}" 2>/dev/null || warn "Code signing failed (non-fatal)"
    fi

    info "Installing to ${INSTALL_DIR}..."
    if [ -w "$INSTALL_DIR" ]; then
        mv "${BINARY}" "${INSTALL_DIR}/${BINARY}"
    else
        sudo mv "${BINARY}" "${INSTALL_DIR}/${BINARY}"
    fi

    info "Installed ${BINARY} ${version} to ${INSTALL_DIR}/${BINARY}"
    return 0
}

install_with_go() {
    if ! command -v go &>/dev/null; then
        return 1
    fi

    info "Installing with go install..."
    go install "github.com/fission-ai/detergent/cmd/detergent@latest"

    local gobin
    gobin=$(go env GOBIN 2>/dev/null)
    if [ -z "$gobin" ]; then
        gobin="$(go env GOPATH)/bin"
    fi

    info "Installed ${BINARY} to ${gobin}/${BINARY}"
    return 0
}

install_from_source() {
    if ! command -v go &>/dev/null; then
        fatal "Go is required to build from source. Install Go from https://go.dev/dl/"
    fi

    if ! command -v git &>/dev/null; then
        fatal "Git is required to build from source."
    fi

    info "Building from source..."

    local tmpdir
    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT

    git clone "https://github.com/${REPO}.git" "${tmpdir}/${BINARY}"
    cd "${tmpdir}/${BINARY}"

    local version
    version=$(git describe --tags --always 2>/dev/null || echo "dev")

    CGO_ENABLED=0 go build \
        -ldflags "-s -w -X github.com/fission-ai/detergent/internal/cli.Version=${version}" \
        -o "${BINARY}" \
        ./cmd/detergent

    chmod +x "${BINARY}"

    if [ -w "$INSTALL_DIR" ]; then
        mv "${BINARY}" "${INSTALL_DIR}/${BINARY}"
    else
        sudo mv "${BINARY}" "${INSTALL_DIR}/${BINARY}"
    fi

    info "Installed ${BINARY} ${version} to ${INSTALL_DIR}/${BINARY}"
    return 0
}

check_path() {
    if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
        warn "${INSTALL_DIR} is not in your PATH"
        warn "Add it with: export PATH=\"${INSTALL_DIR}:\$PATH\""
    fi
}

main() {
    info "Installing ${BINARY}..."

    local platform
    platform=$(detect_platform)
    info "Detected platform: ${platform}"

    # Try GitHub release first
    if install_from_release "$platform"; then
        check_path
        return 0
    fi

    # Fallback: go install
    warn "Release download failed, trying go install..."
    if install_with_go; then
        return 0
    fi

    # Last resort: clone and build
    warn "go install failed, building from source..."
    install_from_source
    check_path
}

main "$@"
