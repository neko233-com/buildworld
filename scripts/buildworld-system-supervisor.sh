#!/bin/sh
# Root-owned, argument-free bridge for the one BuildWorld system service.
set -eu
[ "$#" -eq 0 ] || exit 2
IFS= read -r action || exit 2
case "$action" in
  status) exec /bin/launchctl print system/com.buildworld.server ;;
  stop) exec /bin/launchctl bootout system/com.buildworld.server ;;
  start) exec /bin/launchctl bootstrap system /Library/LaunchDaemons/com.buildworld.server.plist ;;
  *) exit 2 ;;
esac
