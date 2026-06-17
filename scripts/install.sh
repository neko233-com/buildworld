#!/bin/bash
set -e

REPO="neko233-com/buildworld233"
BINARY="buildworld233"
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
    echo "Run: buildworld233 start"
    echo "Status: buildworld233 status"
    echo "Enable boot autostart: buildworld233 enable-autostart"
    echo "Change port: buildworld233 set-port 6050"
    echo "Self update: buildworld233 update"
    echo "Hot reload config: buildworld233 reload-config"
    echo "Export backup: buildworld233 backup export --output ./backup.zip"
    echo "Import backup: buildworld233 backup import --input ./backup.zip"
    echo "Generate worker token: buildworld233 worker generate-token"
    echo "List workers: buildworld233 worker list"
}

if [ "$VERSION" = "latest" ]; then
    VERSION=$(get_latest_version)
fi
VERSION="${VERSION#v}"
echo "Installing buildworld233 v$VERSION ..."
install "$VERSION"
