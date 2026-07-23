#!/usr/bin/env sh
set -eu

bundle="${1:-}"
expected_checksum="${2:-}"
version="${3:-}"
operation_id="${4:-}"
status_path="${5:-}"
lock_path="${6:-}"
install_dir="${7:-}"
config_path="${8:-}"

previous_dir="${install_dir}.previous"
operation_dir="$(dirname "$bundle")"
stage="$operation_dir/install"
activated=0
rotated=0
was_running=0
had_autostart=0
bin_link=

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g; s/	/\\t/g'
}

write_status() {
  state="$1"
  message="$2"
  now="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  temporary="${status_path}.tmp.$$"
  umask 077
  printf '{"status":"%s","operation_id":"%s","version":"%s","message":"%s","updated_at":"%s"}\n' \
    "$(json_escape "$state")" "$(json_escape "$operation_id")" "$(json_escape "$version")" \
    "$(json_escape "$message")" "$now" > "$temporary"
  mv "$temporary" "$status_path"
}

finish() {
  rm -f "$bundle"
  rm -f "$lock_path"
}
trap 'finish' EXIT INT TERM

fail() {
  write_status "failed" "$1"
  echo "$1" >&2
  exit 1
}

case "$version" in
  *[!0-9.]*|*.*.*.*|"") fail "invalid update version" ;;
esac
printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || fail "invalid update version"
printf '%s\n' "$expected_checksum" | grep -Eq '^[a-f0-9]{64}$' || fail "invalid update checksum"

case "$install_dir" in
  ""|"/"|"$HOME") fail "unsafe installation directory" ;;
esac
[ -d "$install_dir" ] || fail "current installation directory is missing"
[ -x "$install_dir/buildworld" ] || fail "current BuildWorld CLI is missing"
[ -f "$bundle" ] || fail "staged update bundle is missing"

file_checksum() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

actual_checksum="$(file_checksum "$bundle")"
[ "$actual_checksum" = "$expected_checksum" ] || fail "staged update checksum mismatch"

archive_entries="$(tar -tzf "$bundle")" || fail "cannot inspect update bundle"
printf '%s\n' "$archive_entries" | awk '
  {
    name=$0
    sub(/^\.\//, "", name)
    if (name ~ /^\// || name ~ /(^|\/)\.\.(\/|$)/ || name ~ /\\/) exit 1
  }
' || fail "update bundle contains unsafe paths"

rm -rf "$stage"
mkdir -p "$stage"
tar -xzf "$bundle" -C "$stage" || fail "cannot extract update bundle"
for required in \
  buildworld buildworld-server buildworld-worker apply-update.sh \
  web/dist/index.html sdk/pipeline/index.d.ts sdk/pipeline/index.js sdk/pipeline/package.json
do
  [ -f "$stage/$required" ] || fail "update bundle is incomplete"
done
chmod 755 "$stage/buildworld" "$stage/buildworld-server" "$stage/buildworld-worker" "$stage/apply-update.sh"

if command -v buildworld >/dev/null 2>&1; then
  bin_link="$(command -v buildworld)"
else
  bin_link="$HOME/.local/bin/buildworld"
fi

case "$(uname -s)" in
  Darwin)
    [ -f "$HOME/Library/LaunchAgents/com.buildworld.server.plist" ] && had_autostart=1
    ;;
  Linux)
    [ -f "$HOME/.config/systemd/user/buildworld.service" ] && had_autostart=1
    ;;
  *) fail "system update is unsupported on this platform" ;;
esac

run_cli() {
  cli="$1"
  shift
  if [ -n "$config_path" ]; then
    "$cli" --config "$config_path" "$@"
  else
    "$cli" "$@"
  fi
}

if run_cli "$install_dir/buildworld" status 2>&1 | grep -q "is running"; then
  was_running=1
fi

restore_previous() {
  rollback_failed=0
  if [ "$activated" = "1" ] && [ -x "$install_dir/buildworld" ]; then
    run_cli "$install_dir/buildworld" pause >/dev/null 2>&1 || rollback_failed=1
  fi
  if [ "$activated" = "1" ] && [ -d "$install_dir" ]; then
    rm -rf "$install_dir" || rollback_failed=1
  fi
  if [ "$rotated" = "1" ] && [ -d "$previous_dir" ]; then
    mv "$previous_dir" "$install_dir" || rollback_failed=1
    if [ "$bin_link" != "$install_dir/buildworld" ]; then
      mkdir -p "$(dirname "$bin_link")" || rollback_failed=1
      ln -sf "$install_dir/buildworld" "$bin_link" || rollback_failed=1
    fi
    if [ "$was_running" = "1" ]; then
      if [ "$had_autostart" = "1" ]; then
        run_cli "$install_dir/buildworld" enable-autostart >/dev/null 2>&1 || rollback_failed=1
      else
        run_cli "$install_dir/buildworld" start >/dev/null 2>&1 || rollback_failed=1
      fi
    fi
  fi
  return "$rollback_failed"
}

apply_update() {
  write_status "applying" "verified update is being installed"
  if [ "$was_running" = "1" ]; then
    run_cli "$install_dir/buildworld" pause
  fi
  sleep 1
  rm -rf "$previous_dir"
  mv "$install_dir" "$previous_dir"
  rotated=1
  mv "$stage" "$install_dir"
  activated=1
  if [ "$bin_link" != "$install_dir/buildworld" ]; then
    mkdir -p "$(dirname "$bin_link")"
    ln -sf "$install_dir/buildworld" "$bin_link"
  fi
  if [ "$had_autostart" = "1" ]; then
    run_cli "$install_dir/buildworld" enable-autostart
  elif [ "$was_running" = "1" ]; then
    run_cli "$install_dir/buildworld" start
  fi

  attempt=0
  while [ "$attempt" -lt 30 ]; do
    if health="$(curl -fsS --max-time 2 http://127.0.0.1:8080/api/health 2>/dev/null)" &&
       printf '%s' "$health" | grep -q '"status":"ok"'; then
      running_version="$(curl -fsS --max-time 2 http://127.0.0.1:8080/api/version 2>/dev/null || true)"
      printf '%s' "$running_version" | grep -q "\"version\":\"v\\{0,1\\}$version\"" || return 1
      return 0
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
  return 1
}

if apply_update; then
  write_status "succeeded" "BuildWorld update completed"
  exit 0
fi

if restore_previous; then
  write_status "rolled_back" "update failed and the previous installation was restored"
else
  write_status "failed" "update failed and rollback requires manual repair"
fi
exit 1
