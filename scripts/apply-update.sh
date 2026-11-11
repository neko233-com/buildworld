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
snapshot_dir="${install_dir}.snapshot-${operation_id}"
operation_dir="$(dirname "$bundle")"
stage="$operation_dir/install"
activated=0
rotated=0
was_running=0
had_autostart=0
system_supervised=0
pause_attempted=0
bin_link=
ledger_before=

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
printf '%s\n' "$operation_id" | grep -Eq '^[a-f0-9]+$' || fail "invalid update operation id"

case "$install_dir" in
  ""|"/"|"$HOME") fail "unsafe installation directory" ;;
esac
[ -d "$install_dir" ] || fail "current installation directory is missing"
[ -x "$install_dir/buildworld" ] || fail "current BuildWorld CLI is missing"
[ -f "$bundle" ] || fail "staged update bundle is missing"
if [ -e "$previous_dir" ] || [ -L "$previous_dir" ]; then
  [ -d "$previous_dir" ] && [ ! -L "$previous_dir" ] || fail "previous installation is not a directory"
  [ ! -e "$snapshot_dir" ] && [ ! -L "$snapshot_dir" ] || fail "rollback snapshot already exists"
fi

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
    system_plist=/Library/LaunchDaemons/com.buildworld.server.plist
    system_bridge=/usr/local/libexec/buildworld-system-supervisor
    system_info="$(/bin/launchctl list com.buildworld.server 2>/dev/null || true)"
    if [ -e "$system_plist" ] && [ -z "$system_info" ]; then
      fail "cannot identify the configured system supervisor"
    fi
    if printf '%s\n' "$system_info" | grep -q '"LimitLoadToSessionType" = "System"'; then
      # The root bridge has exactly one executable entry and three fixed
      # actions. Verify both it and the daemon before any service is stopped.
      for protected in "$system_plist" "$system_bridge"; do
        [ -f "$protected" ] && [ ! -L "$protected" ] || fail "system supervisor file is missing or unsafe"
        [ "$(/usr/bin/stat -f '%u' "$protected")" = 0 ] || fail "system supervisor file is not root owned"
        mode="$(/usr/bin/stat -f '%Lp' "$protected")"
        [ "$((0$mode & 022))" -eq 0 ] || fail "system supervisor file is writable by other users"
      done
      bridge_hash="$(/usr/bin/shasum -a 256 "$system_bridge")" || fail "cannot check system supervisor bridge"
      [ "${bridge_hash%% *}" = "3b8e7e8841bf5dcdc4a058b2978162912ea53913f91c6ab9ad560db03a9aa15c" ] || fail "system supervisor bridge identity mismatch"
      [ "$(/usr/libexec/PlistBuddy -c 'Print :UserName' "$system_plist")" = "$(id -un)" ] || fail "system supervisor user mismatch"
      [ "$(/usr/libexec/PlistBuddy -c 'Print :RunAtLoad' "$system_plist")" = true ] || fail "system supervisor does not start at load"
      [ "$(/usr/libexec/PlistBuddy -c 'Print :KeepAlive' "$system_plist")" = true ] || fail "system supervisor does not keep the service alive"
      system_pid="$(printf '%s\n' "$system_info" | awk -F= '/"PID" =/ { gsub(/[;[:space:]]/, "", $2); print $2; exit }')"
      case "$system_pid" in ''|*[!0-9]*) fail "system supervisor PID is invalid" ;; esac
      [ "$system_pid" -gt 0 ] || fail "system supervisor PID is invalid"
      system_executable="$(/bin/ps -p "$system_pid" -o comm= | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
      [ "$system_executable" = "$install_dir/buildworld-server" ] || fail "system supervisor process identity mismatch"
      bridge_status="$(printf 'status\n' | /usr/bin/sudo -n "$system_bridge")" || fail "system supervisor bridge is unavailable"
      bridge_pid="$(printf '%s\n' "$bridge_status" | awk -F= '/^[[:space:]]*pid =/ { gsub(/[[:space:]]/, "", $2); print $2; exit }')"
      [ "$bridge_pid" = "$system_pid" ] || fail "system supervisor status identity mismatch"
      system_supervised=1
    fi
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

stop_server() {
  if [ "$system_supervised" = 1 ]; then
    if printf 'status\n' | /usr/bin/sudo -n "$system_bridge" >/dev/null 2>&1; then
      printf 'stop\n' | /usr/bin/sudo -n "$system_bridge"
    else
      # An unavailable bridge is not evidence of a stopped daemon. Refuse to
      # replace binaries while either launchd or the control port is present.
      ! /bin/launchctl list com.buildworld.server >/dev/null 2>&1 || return 1
      ! /usr/bin/nc -z -w 2 127.0.0.1 8080 >/dev/null 2>&1
    fi
  else
    run_cli "$install_dir/buildworld" pause
  fi
}

start_server() {
  if [ "$system_supervised" = 1 ]; then
    printf 'start\n' | /usr/bin/sudo -n "$system_bridge"
  elif [ "$had_autostart" = 1 ]; then
    run_cli "$install_dir/buildworld" enable-autostart
  else
    run_cli "$install_dir/buildworld" start
  fi
}

if run_cli "$install_dir/buildworld" status 2>&1 | grep -q "is running"; then
  was_running=1
fi
if [ "$system_supervised" = 1 ] && [ "$was_running" != 1 ]; then
  fail "system-supervised controller is not healthy; refusing update"
fi

restore_previous() {
  rollback_failed=0
  if [ "$activated" = "1" ] && [ -x "$install_dir/buildworld" ]; then
    # Never replace a possibly running executable or return an older binary to
    # a database whose migration ledger changed during the attempted start.
    stop_server >/dev/null 2>&1 || return 1
    ledger_after="$(run_cli "$install_dir/buildworld" inspect-migration-ledger 2>/dev/null)" || return 1
    [ -n "$ledger_before" ] && [ "$ledger_after" = "$ledger_before" ] || return 1
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
  fi
  if [ "$pause_attempted" = "1" ] && [ "$was_running" = "1" ]; then
    # A failed pause may leave the original server running under an external
    # supervisor. Do not start a second server or report a false repair need.
    current_version="$(run_cli "$install_dir/buildworld" version 2>/dev/null | awk '{print $NF}')"
    running_version="$(curl -fsS --max-time 2 http://127.0.0.1:8080/api/version 2>/dev/null || true)"
    health="$(curl -fsS --max-time 2 http://127.0.0.1:8080/api/health 2>/dev/null || true)"
    if [ "$activated" = "0" ] && printf '%s' "$current_version" | grep -Eq '^v?[0-9]+\.[0-9]+\.[0-9]+$' &&
       printf '%s' "$running_version" | grep -Fq "\"version\":\"$current_version\"" &&
       printf '%s' "$health" | grep -q '"status":"ok"'; then
      :
    else
      start_server >/dev/null 2>&1 || rollback_failed=1
    fi
    # A successful start command does not prove that the restored server can
    # open the current database or remain healthy after an upgrade migration.
    attempt=0
    restored_healthy=0
    while [ "$rollback_failed" = "0" ] && [ "$attempt" -lt 30 ]; do
      health="$(curl -fsS --max-time 2 http://127.0.0.1:8080/api/health 2>/dev/null || true)"
      running_version="$(curl -fsS --max-time 2 http://127.0.0.1:8080/api/version 2>/dev/null || true)"
      if printf '%s' "$current_version" | grep -Eq '^v?[0-9]+\.[0-9]+\.[0-9]+$' &&
         printf '%s' "$running_version" | grep -Fq "\"version\":\"$current_version\"" &&
         printf '%s' "$health" | grep -q '"status":"ok"'; then
        restored_healthy=1
        break
      fi
      attempt=$((attempt + 1))
      sleep 1
    done
    [ "$restored_healthy" = "1" ] || rollback_failed=1
  fi
  return "$rollback_failed"
}

apply_update() {
  write_status "applying" "verified update is being installed" || return 1
  if [ "$was_running" = "1" ]; then
    pause_attempted=1
    stop_server || return 1
  fi
  # The incoming CLI can inspect databases from older installations without
  # applying migrations. If the database cannot be identified, fail closed.
  ledger_before="$(run_cli "$stage/buildworld" inspect-migration-ledger)" || return 1
  printf '%s\n' "$ledger_before" | grep -Eq '^v1:[0-9a-f]{64}$' || return 1
  sleep 1
  if [ -e "$previous_dir" ] || [ -L "$previous_dir" ]; then
    [ -d "$previous_dir" ] && [ ! -L "$previous_dir" ] || return 1
    [ ! -e "$snapshot_dir" ] && [ ! -L "$snapshot_dir" ] || return 1
    mv "$previous_dir" "$snapshot_dir" || return 1
  fi
  mv "$install_dir" "$previous_dir" || return 1
  rotated=1
  mv "$stage" "$install_dir" || return 1
  activated=1
  if [ "$bin_link" != "$install_dir/buildworld" ]; then
    mkdir -p "$(dirname "$bin_link")" || return 1
    ln -sf "$install_dir/buildworld" "$bin_link" || return 1
  fi
  if [ "$system_supervised" = "1" ] || [ "$had_autostart" = "1" ]; then
    start_server || return 1
  elif [ "$was_running" = "1" ]; then
    start_server || return 1
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
