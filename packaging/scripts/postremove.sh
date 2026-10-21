#!/bin/sh
set -eu

# Skipped where systemd is not running (containers, chroots, image builds).
if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  systemctl daemon-reload || true
fi

case "${1:-}" in
  # Debian: purge removes everything reconD left on the host, including the
  # administrator's config and deployment state. RPM has no purge; its
  # uninstall keeps both.
  purge)
    rm -rf /etc/recond /var/lib/recond
    rm -f /run/recond.preremove-state
    ;;
  # Debian: the removal finished, so the saved pre-removal state is not needed.
  remove)
    rm -f /run/recond.preremove-state
    ;;
  # RPM: uninstall finished.
  0)
    rm -f /run/recond.preremove-state
    ;;
esac
