#!/bin/sh
set -eu

# Records the service state before removal so postinstall.sh can restore it
# if dpkg abandons the removal (abort-remove).
saved_state=/run/recond.preremove-state

case "${1:-}" in
  remove|0)
    if command -v systemctl >/dev/null 2>&1; then
      true > "$saved_state" 2>/dev/null || true
      if systemctl is-enabled --quiet recond.service 2>/dev/null; then
        echo enabled >> "$saved_state" || true
      fi
      if systemctl is-active --quiet recond.service 2>/dev/null; then
        echo active >> "$saved_state" || true
      fi

      # Stopping needs a running systemd; disabling only edits unit symlinks.
      if [ -d /run/systemd/system ]; then
        systemctl stop recond.service || true
      fi
      systemctl disable recond.service || true
    fi
    ;;
esac
