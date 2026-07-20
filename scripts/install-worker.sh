#!/bin/bash
set -e

REPO="neko233-com/buildworld"
BINARY="buildworld-worker"
VERSION="${1:-latest}"

get_latest_version() {
    curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/' || echo "0.1.0"
}

install() {
    local ver="$1"
    local os=$(uname -s | tr '[:upper:]' '[:lower:]')
    local arch=$(uname -m)
    case "$arch" in
        x86_64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
    esac
    local asset="${BINARY}-${os}-${arch}"
    local url="https://github.com/$REPO/releases/download/v$ver/$asset"
    local install_dir="/usr/local/bin"
    echo "Downloading $url ..."
    sudo curl -fsSL "$url" -o "$install_dir/$BINARY"
    sudo chmod +x "$install_dir/$BINARY"
    echo "Installed to $install_dir/$BINARY"
    echo "Register worker on server: buildworld-worker register --server http://SERVER:8700 --name $(hostname)"
}

if [ "$VERSION" = "latest" ]; then
    VERSION=$(get_latest_version)
fi
VERSION="${VERSION#v}"
echo "Installing buildworld-worker v$VERSION ..."
install "$VERSION"
