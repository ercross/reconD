#!/bin/sh
set -eu

# Written by preremove.sh; used to restore the service if a removal is abandoned.
saved_state=/run/recond.preremove-state

# systemctl is installed but systemd is not running in containers, chroots and
# image builds. Commands that talk to the running manager are skipped there;
# enable/disable only edit unit symlinks, so they run whenever systemctl exists.
has_systemctl() {
  command -v systemctl >/dev/null 2>&1
}

systemd_running() {
  has_systemctl && [ -d /run/systemd/system ]
}

print_setup_hint() {
  if [ ! -e /etc/recond/config.yaml ]; then
    cat <<'EOF'

reconD is installed but not running. To start it:

  1. Create a config from the example:
       sudo cp /etc/recond/config.yaml.example /etc/recond/config.yaml
       sudo chmod 600 /etc/recond/config.yaml
       sudoedit /etc/recond/config.yaml

  2. Enable and start the service:
       sudo systemctl enable --now recond

  3. Check that it is running:
       sudo systemctl status recond
       sudo journalctl -u recond -f

Documentation: https://github.com/ercross/reconD#installation

EOF
  fi
}

restore_after_abort_remove() {
  if [ -f "$saved_state" ]; then
    if grep -qx enabled "$saved_state"; then
      systemctl enable recond.service || true
    fi
    if grep -qx active "$saved_state" && systemd_running; then
      systemctl start recond.service || true
    fi
  fi
  rm -f "$saved_state" || true
}

if systemd_running; then
  systemctl daemon-reload || true
fi

case "${1:-}" in
  # Debian: fresh install has no previous version; upgrade passes it. So does a
  # reinstall after `apt remove`, which leaves the service disabled.
  configure)
    if [ -n "${2:-}" ]; then
      if systemd_running; then
        systemctl try-restart recond.service || true
      fi
    fi
    print_setup_hint
    ;;
  # Debian: a removal failed after preremove.sh stopped and disabled the service.
  abort-remove)
    if has_systemctl; then
      restore_after_abort_remove
    fi
    ;;
  ''|*[!0-9]*) ;;
  # RPM: installed package count; 1 is a fresh install, more is an upgrade.
  *)
    if [ "$1" -gt 1 ]; then
      if systemd_running; then
        systemctl try-restart recond.service || true
      fi
    fi
    print_setup_hint
    ;;
esac
