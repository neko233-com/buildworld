#!/usr/bin/env sh
set -eu

REPO="neko233-com/buildworld"
VERSION="${1:-latest}"
NO_AUTOSTART="${BUILDWORLD_NO_AUTOSTART:-0}"
GITHUB_AUTH_TOKEN="${GH_TOKEN:-${GITHUB_TOKEN:-}}"
GITHUB_MIRROR="${BUILDWORLD_GITHUB_MIRROR:-https://gh-proxy.com}"
install_dir="${BUILDWORLD_INSTALL_DIR:-$HOME/.local/lib/buildworld}"
bin_dir="${BUILDWORLD_BIN_DIR:-$HOME/.local/bin}"
previous_dir="${install_dir}.previous"
tmp="$(mktemp -d)"
release_metadata="$tmp/release.json"
transaction_error=
rotated=0
installed_new=0
had_existing_install=0
had_autostart=0
was_running=0

cleanup() { rm -rf "$tmp"; }
trap 'cleanup' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if [ -z "$GITHUB_AUTH_TOKEN" ] && command -v gh >/dev/null 2>&1; then
  if candidate="$(gh auth token 2>/dev/null)"; then
    GITHUB_AUTH_TOKEN="$candidate"
  fi
fi

case "$GITHUB_MIRROR" in
  ""|off|none) GITHUB_MIRROR= ;;
  https://*)
    case "$GITHUB_MIRROR" in *[[:space:]@?#]*) echo "BUILDWORLD_GITHUB_MIRROR must not contain credentials, query parameters, or fragments" >&2; exit 1 ;; esac
    ;;
  *) echo "BUILDWORLD_GITHUB_MIRROR must be an https URL, off, or none" >&2; exit 1 ;;
esac

github_source_url() {
  uri="$1"
  if [ -n "$GITHUB_MIRROR" ]; then
    printf '%s/%s' "${GITHUB_MIRROR%/}" "$uri"
  else
    printf '%s' "$uri"
  fi
}

github_api_to_file() {
  uri="$1"
  destination="$2"
  source_uri="$(github_source_url "$uri")"
  rm -f "$destination"
  if [ -n "$GITHUB_AUTH_TOKEN" ] && [ -z "$GITHUB_MIRROR" ]; then
    curl -fsSL \
      -H "Authorization: Bearer $GITHUB_AUTH_TOKEN" \
      -H "Accept: application/vnd.github+json" \
      -H "X-GitHub-Api-Version: 2022-11-28" \
      "$source_uri" -o "$destination"
  else
    curl -fsSL \
      -H "Accept: application/vnd.github+json" \
      -H "X-GitHub-Api-Version: 2022-11-28" \
      "$source_uri" -o "$destination"
  fi
}

ensure_release_metadata() {
  [ -s "$release_metadata" ] && return 0
  github_api_to_file "https://api.github.com/repos/$REPO/releases/tags/v$VERSION" "$release_metadata"
}

asset_api_url() {
  wanted="$1"
  tr -d '\r\n' < "$release_metadata" |
    sed 's/{[[:space:]]*"url"[[:space:]]*:[[:space:]]*/\
{"url":/g' |
    awk -v wanted="$wanted" '
    (index($0, "\"name\":\"" wanted "\"") || index($0, "\"name\": \"" wanted "\"")) &&
      index($0, "/releases/assets/") {
      value = $0
      sub(/^\{"url":"/, "", value)
      sub(/".*$/, "", value)
      print value
      exit
    }
  '
}

download_api_asset() {
  api_url="$1"
  destination="$2"
  source_uri="$(github_source_url "$api_url")"
  if [ -n "$GITHUB_AUTH_TOKEN" ] && [ -z "$GITHUB_MIRROR" ]; then
    curl -fsSL \
      -H "Authorization: Bearer $GITHUB_AUTH_TOKEN" \
      -H "Accept: application/octet-stream" \
      -H "X-GitHub-Api-Version: 2022-11-28" \
      "$source_uri" -o "$destination"
  else
    curl -fsSL \
      -H "Accept: application/octet-stream" \
      -H "X-GitHub-Api-Version: 2022-11-28" \
      "$source_uri" -o "$destination"
  fi
}

download_release_asset() {
  name="$1"
  destination="$2"
  quiet="${3:-0}"
  direct="$(github_source_url "https://github.com/$REPO/releases/download/v$VERSION/$name")"

  if [ -n "$GITHUB_AUTH_TOKEN" ] && [ -z "$GITHUB_MIRROR" ]; then
    if ensure_release_metadata; then
      api_url="$(asset_api_url "$name")"
      if [ -n "$api_url" ] && download_api_asset "$api_url" "$destination"; then
        return 0
      fi
    fi
    rm -f "$destination"
  fi

  if curl -fsSL "$direct" -o "$destination"; then
    return 0
  fi
  rm -f "$destination"

  if { [ -z "$GITHUB_AUTH_TOKEN" ] || [ -n "$GITHUB_MIRROR" ]; } && ensure_release_metadata; then
    api_url="$(asset_api_url "$name")"
    if [ -n "$api_url" ] && download_api_asset "$api_url" "$destination"; then
      return 0
    fi
    rm -f "$destination"
  fi

  if [ "$quiet" != "1" ]; then
    if [ -z "$GITHUB_AUTH_TOKEN" ]; then
      echo "Cannot download $name. Check GitHub connectivity or set BUILDWORLD_GITHUB_MIRROR=off for direct access." >&2
    else
      echo "Cannot download $name. Check that the GitHub token can read releases for $REPO." >&2
    fi
  fi
  return 1
}

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in linux|darwin) ;; *) echo "Unsupported operating system: $os" >&2; exit 1 ;; esac
case "$(uname -m)" in x86_64|amd64) arch="amd64" ;; aarch64|arm64) arch="arm64" ;; *) echo "Unsupported architecture" >&2; exit 1 ;; esac

if [ "$VERSION" = "latest" ]; then
  github_api_to_file "https://api.github.com/repos/$REPO/releases/latest" "$release_metadata" || {
    echo "Cannot resolve the latest release. For a private repository, set GH_TOKEN/GITHUB_TOKEN or run 'gh auth login'." >&2
    exit 1
  }
  VERSION="$(sed -n 's/.*"tag_name": *"v\([^"]*\)".*/\1/p' "$release_metadata" | head -n 1)"
else
  VERSION="${VERSION#v}"
fi
printf '%s\n' "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || {
  echo "Invalid BuildWorld version: $VERSION" >&2
  exit 1
}

asset="buildworld-$os-$arch.tar.gz"
echo "Downloading BuildWorld v$VERSION for $os/$arch ..."
download_release_asset "checksums.txt" "$tmp/checksums.txt"
expected="$(tr -d '\r' < "$tmp/checksums.txt" | awk -v asset="$asset" '$2 == asset {print $1}' | tr -d '\r')"
[ -n "$expected" ] || { echo "Missing checksum for $asset" >&2; exit 1; }

file_checksum() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}' | tr -d '\r'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}' | tr -d '\r'
  else
    echo "Neither sha256sum nor shasum is available" >&2
    return 1
  fi
}

download_bundle() {
  parts="$(tr -d '\r' < "$tmp/checksums.txt" | awk -v prefix="$asset.part" 'index($2, prefix) == 1 {print $2}' | tr -d '\r' | sort | tr -d '\r')"
  [ -n "$parts" ] || { echo "Missing multipart release bundle for $asset" >&2; return 1; }
  : > "$tmp/$asset"
  index=0
  for part in $parts; do
    expected_part="$asset.part$(printf '%03d' "$index")"
    [ "$part" = "$expected_part" ] || {
      echo "Multipart bundle is incomplete: expected $expected_part, found $part" >&2
      return 1
    }
    echo "Downloading $part ..."
    download_release_asset "$part" "$tmp/$part" || return 1
    part_expected="$(tr -d '\r' < "$tmp/checksums.txt" | awk -v part="$part" '$2 == part {print $1}' | tr -d '\r')"
    [ -n "$part_expected" ] || { echo "Missing checksum for $part" >&2; return 1; }
    part_actual="$(file_checksum "$tmp/$part")" || return 1
    [ "$part_actual" = "$part_expected" ] || { echo "Checksum mismatch for $part" >&2; return 1; }
    cat "$tmp/$part" >> "$tmp/$asset" || return 1
    rm -f "$tmp/$part" || return 1
    index=$((index + 1))
  done
}

download_bundle
actual="$(file_checksum "$tmp/$asset")"
[ "$actual" = "$expected" ] || { echo "Checksum mismatch for $asset" >&2; exit 1; }

stage="$tmp/install"
mkdir -p "$stage" "$bin_dir" "$(dirname "$install_dir")"
tar -xzf "$tmp/$asset" -C "$stage"
chmod +x "$stage/buildworld" "$stage/buildworld-server" "$stage/buildworld-worker" "$stage/apply-update.sh"

autostart_registered() {
  case "$os" in
    darwin) [ -f "$HOME/Library/LaunchAgents/com.buildworld.server.plist" ] ;;
    linux) [ -f "$HOME/.config/systemd/user/buildworld.service" ] ;;
  esac
}

if autostart_registered; then had_autostart=1; fi
if [ -e "$install_dir" ] || [ -L "$install_dir" ]; then had_existing_install=1; fi

old_cli=
if [ -x "$install_dir/buildworld" ]; then
  old_cli="$install_dir/buildworld"
elif [ -x "$bin_dir/buildworld" ]; then
  old_cli="$bin_dir/buildworld"
elif command -v buildworld >/dev/null 2>&1; then
  old_cli="$(command -v buildworld)"
fi

cli_is_running() {
  cli="$1"
  if ! status_output="$("$cli" status 2>&1)"; then
    echo "Cannot inspect the existing BuildWorld service: $status_output" >&2
    return 2
  fi
  case "$status_output" in *"is running"*) return 0 ;; *) return 1 ;; esac
}

wait_for_cli_running() {
  wait_cli="$1"
  wait_attempt=0
  while [ "$wait_attempt" -lt 10 ]; do
    if cli_is_running "$wait_cli"; then
      return 0
    fi
    wait_attempt=$((wait_attempt + 1))
    sleep 1
  done
  return 1
}

restore_paused_previous_service() {
  [ "$was_running" = "1" ] || return 0
  restore_cli="$old_cli"
  if [ -z "$restore_cli" ] && [ -x "$install_dir/buildworld" ]; then
    restore_cli="$install_dir/buildworld"
  fi
  if [ -n "$restore_cli" ] && [ -x "$restore_cli" ]; then
    if [ "$had_autostart" = "1" ]; then
      "$restore_cli" enable-autostart >/dev/null 2>&1 || return 1
      wait_for_cli_running "$restore_cli"
      return $?
    fi
    "$restore_cli" start >/dev/null 2>&1
    return $?
  fi
  [ "$had_autostart" = "1" ] || return 1
  case "$os" in
    linux)
      systemctl --user daemon-reload >/dev/null 2>&1 &&
        systemctl --user start buildworld.service >/dev/null 2>&1
      ;;
    darwin)
      restore_uid="$(id -u)"
      restore_plist="$HOME/Library/LaunchAgents/com.buildworld.server.plist"
      if launchctl print "gui/$restore_uid/com.buildworld.server" >/dev/null 2>&1; then
        return 0
      fi
      launchctl bootstrap "gui/$restore_uid" "$restore_plist" >/dev/null 2>&1
      ;;
  esac
}

interrupt_before_install() {
  interrupt_code="$1"
  trap - INT TERM
  if ! restore_paused_previous_service; then
    echo "Installation was interrupted and the previous service could not be restarted automatically." >&2
  fi
  exit "$interrupt_code"
}

# No bundle has been rotated yet, but the existing service can be paused below.
trap 'interrupt_before_install 130' INT
trap 'interrupt_before_install 143' TERM

if [ -n "$old_cli" ]; then
  if cli_is_running "$old_cli"; then
    was_running=1
    echo "Pausing running BuildWorld service ..."
    "$old_cli" pause
    if cli_is_running "$old_cli"; then
      echo "BuildWorld is still running after pause" >&2
      exit 1
    else
      status_code=$?
      [ "$status_code" -eq 1 ] || exit "$status_code"
    fi
  else
    status_code=$?
    [ "$status_code" -eq 1 ] || exit "$status_code"
  fi
elif [ "$had_autostart" = "1" ]; then
  echo "Stopping registered BuildWorld service ..."
  case "$os" in
    linux)
      systemctl --user daemon-reload
      if systemctl --user is-active --quiet buildworld.service; then
        was_running=1
      else
        status_code=$?
        [ "$status_code" -eq 3 ] || [ "$status_code" -eq 4 ] || exit "$status_code"
      fi
      systemctl --user stop buildworld.service
      ;;
    darwin)
      uid="$(id -u)"
      plist="$HOME/Library/LaunchAgents/com.buildworld.server.plist"
      if launchctl print "gui/$uid/com.buildworld.server" >/dev/null 2>&1; then
        was_running=1
        launchctl bootout "gui/$uid" "$plist"
      fi
      ;;
  esac
fi

persist_path_profile() {
  profile="$1"
  mkdir -p "$(dirname "$profile")" || return 1
  touch "$profile" || return 1
  grep -F "export BUILDWORLD_BIN_DIR=\"$bin_dir\"" "$profile" >/dev/null 2>&1 ||
    printf '\nexport BUILDWORLD_BIN_DIR="%s"\nexport PATH="$BUILDWORLD_BIN_DIR:$PATH"\n' "$bin_dir" >> "$profile" || return 1
}

ensure_path() {
  case "$os" in
    darwin)
      profile="${ZDOTDIR:-$HOME}/.zprofile"
      persist_path_profile "$profile" || return 1
      ;;
    linux)
      persist_path_profile "$HOME/.profile" || return 1
      case "${SHELL##*/}" in
        zsh)
          persist_path_profile "${ZDOTDIR:-$HOME}/.zprofile" || return 1
          ;;
        bash)
          if [ -e "$HOME/.bash_profile" ]; then
            persist_path_profile "$HOME/.bash_profile" || return 1
          fi
          ;;
      esac
      mkdir -p "$HOME/.config/environment.d" || return 1
      printf 'BUILDWORLD_HOME=%s\nBUILDWORLD_BIN_DIR=%s\nPATH=%s:%s\n' \
        "$install_dir" "$bin_dir" "$bin_dir" "$PATH" > "$HOME/.config/environment.d/90-buildworld.conf" || return 1
      ;;
  esac
  export PATH="$bin_dir:$PATH"
}

install_new_bundle() {
  if [ -e "$install_dir" ] || [ -L "$install_dir" ]; then
    rm -rf "$previous_dir" || { transaction_error="remove old previous bundle"; return 1; }
    rotated=1
    mv "$install_dir" "$previous_dir" || { rotated=0; transaction_error="rotate current bundle"; return 1; }
  fi
  installed_new=1
  mv "$stage" "$install_dir" || { installed_new=0; transaction_error="activate new bundle"; return 1; }
  ln -sf "$install_dir/buildworld" "$bin_dir/buildworld" || { transaction_error="link CLI into PATH"; return 1; }
  ensure_path || { transaction_error="persist CLI PATH"; return 1; }

  if [ "$NO_AUTOSTART" = "1" ]; then
    if autostart_registered; then
      "$install_dir/buildworld" disable-autostart || { transaction_error="disable autostart"; return 1; }
    fi
    "$install_dir/buildworld" start || { transaction_error="start BuildWorld"; return 1; }
  else
    "$install_dir/buildworld" enable-autostart || { transaction_error="enable autostart"; return 1; }
    wait_for_cli_running "$install_dir/buildworld" || { transaction_error="wait for BuildWorld autostart"; return 1; }
  fi
}

rollback_install() {
  rollback_failed=0
  if [ "$installed_new" = "1" ] && [ -x "$install_dir/buildworld" ]; then
    "$install_dir/buildworld" pause >/dev/null 2>&1 || rollback_failed=1
    if [ "$had_autostart" = "0" ] && autostart_registered; then
      "$install_dir/buildworld" disable-autostart >/dev/null 2>&1 || rollback_failed=1
    fi
  fi
  if [ "$installed_new" = "1" ] && { [ -e "$install_dir" ] || [ -L "$install_dir" ]; }; then
    rm -rf "$install_dir" || rollback_failed=1
  fi
  if [ "$rotated" = "1" ] && { [ -e "$previous_dir" ] || [ -L "$previous_dir" ]; }; then
    mv "$previous_dir" "$install_dir" || rollback_failed=1
    ln -sf "$install_dir/buildworld" "$bin_dir/buildworld" || rollback_failed=1
    if [ -x "$install_dir/buildworld" ]; then
      if [ "$had_autostart" = "1" ] && [ "$was_running" = "0" ] && ! autostart_registered; then
        "$install_dir/buildworld" enable-autostart >/dev/null 2>&1 || rollback_failed=1
        "$install_dir/buildworld" pause >/dev/null 2>&1 || rollback_failed=1
      fi
    fi
  elif [ "$had_existing_install" != "1" ]; then
    rm -f "$bin_dir/buildworld" || rollback_failed=1
  fi
  if [ "$was_running" = "1" ]; then
    restore_paused_previous_service || rollback_failed=1
  fi
  return "$rollback_failed"
}

interrupt_during_install() {
  interrupt_code="$1"
  trap - INT TERM
  echo "BuildWorld installation was interrupted; restoring the previous state." >&2
  if ! rollback_install; then
    echo "Rollback was incomplete; inspect $install_dir and $previous_dir." >&2
  fi
  exit "$interrupt_code"
}

trap 'interrupt_during_install 130' INT
trap 'interrupt_during_install 143' TERM

if ! install_new_bundle; then
  echo "BuildWorld installation failed while attempting to $transaction_error." >&2
  if rollback_install; then
    [ "$rotated" = "1" ] && echo "The previous BuildWorld bundle was restored." >&2
  else
    echo "Rollback was incomplete; inspect $install_dir and $previous_dir." >&2
  fi
  exit 1
fi
trap - INT TERM

if [ "$rotated" = "1" ]; then
  echo "Updated and started BuildWorld v$VERSION. Previous bundle: $previous_dir"
else
  echo "Installed and started BuildWorld v$VERSION."
fi
