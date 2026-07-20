#!/usr/bin/env sh
set -eu

REPO="neko233-com/buildworld233"
VERSION="${1:-latest}"
NO_AUTOSTART="${BUILDWORLD_NO_AUTOSTART:-0}"

latest_version() {
  curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name": *"v\([^"]*\)".*/\1/p' | head -n 1
}

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in linux|darwin) ;; *) echo "Unsupported operating system: $os" >&2; exit 1 ;; esac
case "$(uname -m)" in x86_64|amd64) arch="amd64" ;; aarch64|arm64) arch="arm64" ;; *) echo "Unsupported architecture" >&2; exit 1 ;; esac
if [ "$VERSION" = "latest" ]; then VERSION="$(latest_version)"; fi
VERSION="${VERSION#v}"
asset="buildworld-$os-$arch.tar.gz"
base="https://github.com/$REPO/releases/download/v$VERSION"
tmp="$(mktemp -d)"
install_dir="${BUILDWORLD_INSTALL_DIR:-$HOME/.local/lib/buildworld}"
bin_dir="${BUILDWORLD_BIN_DIR:-$HOME/.local/bin}"
previous_dir="${install_dir}.previous"
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT INT TERM

echo "Downloading BuildWorld v$VERSION for $os/$arch ..."
curl -fsSL "$base/$asset" -o "$tmp/$asset"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"
expected="$(tr -d '\r' < "$tmp/checksums.txt" | awk "\$2 == \"$asset\" {print \$1}")"
[ -n "$expected" ] || { echo "Missing checksum for $asset" >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then actual="$(sha256sum "$tmp/$asset" | awk '{print $1}')"; else actual="$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')"; fi
[ "$actual" = "$expected" ] || { echo "Checksum mismatch for $asset" >&2; exit 1; }

was_running=0
if command -v buildworld >/dev/null 2>&1 && buildworld status 2>/dev/null | grep -q 'is running'; then
  was_running=1
  echo "Pausing running BuildWorld service ..."
  buildworld pause
fi

stage="$tmp/install"
mkdir -p "$stage" "$bin_dir" "$(dirname "$install_dir")"
tar -xzf "$tmp/$asset" -C "$stage"
chmod +x "$stage/buildworld" "$stage/buildworld-server" "$stage/buildworld-worker"
if [ -d "$install_dir" ]; then
  rm -rf "$previous_dir"
  mv "$install_dir" "$previous_dir"
fi
if ! mv "$stage" "$install_dir"; then
  [ -d "$previous_dir" ] && mv "$previous_dir" "$install_dir"
  echo "Install failed; previous BuildWorld bundle restored." >&2
  exit 1
fi
ln -sf "$install_dir/buildworld" "$bin_dir/buildworld"
case "$os" in
  darwin)
    # macOS uses zsh by default. Create the login-shell profile when absent so
    # `buildworld` is available after the next Terminal or SSH login.
    profile="${ZDOTDIR:-$HOME}/.zprofile"
    mkdir -p "$(dirname "$profile")"
    grep -F 'BUILDWORLD_BIN_DIR' "$profile" >/dev/null 2>&1 || printf '\nexport BUILDWORLD_BIN_DIR="%s"\nexport PATH="$BUILDWORLD_BIN_DIR:$PATH"\n' "$bin_dir" >> "$profile"
    ;;
  linux)
    for profile in "$HOME/.profile" "$HOME/.zprofile" "$HOME/.bash_profile"; do
      [ -e "$profile" ] || continue
      grep -F 'BUILDWORLD_BIN_DIR' "$profile" >/dev/null 2>&1 || printf '\nexport BUILDWORLD_BIN_DIR="%s"\nexport PATH="$BUILDWORLD_BIN_DIR:$PATH"\n' "$bin_dir" >> "$profile"
    done
    mkdir -p "$HOME/.config/environment.d"
    printf 'BUILDWORLD_HOME=%s\nBUILDWORLD_BIN_DIR=%s\n' "$install_dir" "$bin_dir" > "$HOME/.config/environment.d/90-buildworld.conf"
    ;;
esac
export PATH="$bin_dir:$PATH"

if [ "$NO_AUTOSTART" != "1" ]; then
  if ! buildworld enable-autostart; then echo "Autostart needs a user service manager; run 'buildworld enable-autostart' from an interactive session." >&2; fi
fi
if ! buildworld start; then
  echo "New version failed health check. Restore previous bundle at: $previous_dir" >&2
  exit 1
fi
if [ "$was_running" = "1" ]; then
  echo "Updated and restarted BuildWorld v$VERSION. Previous bundle: $previous_dir"
else
  echo "Installed BuildWorld v$VERSION and started it. Previous bundle: $previous_dir"
fi
