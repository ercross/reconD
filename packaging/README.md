# Package service lifecycle

The Debian and RPM packages use these hooks:

- `postinstall.sh` reloads systemd's unit definitions.
  - On every install, upgrade, or reinstall, it prints setup directions if
    `/etc/recond/config.yaml` does not exist yet. It never enables or starts
    the service.
  - On upgrade, it runs `systemctl try-restart recond.service` so an active
    agent uses the new binary. A stopped service remains stopped, and its
    enabled/disabled setting is preserved.
  - On Debian `abort-remove` (dpkg calls this when `prerm` fails during
    removal), it restores the enabled/active state that `preremove.sh` saved.
- `preremove.sh` saves whether the service was enabled and active to
  `/run/recond.preremove-state`, then stops and disables it on uninstall,
  before the package removes the service unit. It does nothing during upgrades.
- `postremove.sh` reloads systemd's unit definitions after removal or upgrade,
  and deletes the saved pre-removal state once removal is complete.
  - On Debian `purge`, it deletes `/etc/recond` and `/var/lib/recond`,
    including the administrator's `config.yaml` and deployment state.

Debian signals an upgrade to `postinstall.sh` with `configure` and a previous
version; RPM passes an installed package count greater than one. Uninstall is
`remove` for Debian and `0` for RPM. Other removal actions, including upgrade
paths, do not stop or disable the service.

`/etc/recond/config.yaml.example` is shipped as a regular file, not a config
file, so it is always replaced by the latest example and never causes an
upgrade prompt. Administrators should copy it to `/etc/recond/config.yaml`,
which the package does not own.

Fresh installs do not enable or start the service. Copy
`/etc/recond/config.yaml.example` to `/etc/recond/config.yaml`, customize it,
then run `sudo systemctl enable --now recond.service` when ready.

Only Debian `purge` deletes administrator configuration or deployment state.
RPM uninstall and Debian `remove` keep both.

Reinstalling after an uninstall does not re-enable the service, because
uninstall disabled it. Debian passes the previously configured version to
`postinstall.sh` after `remove`, so the reinstall takes the upgrade path, and
`try-restart` leaves the stopped service stopped. Run
`sudo systemctl enable --now recond.service` to start it again.

Systemctl calls are skipped when the command is unavailable. When systemd is
installed but not running (containers, chroots, image builds), detected by the
absence of `/run/systemd/system`, the hooks skip calls that need the running
manager (`daemon-reload`, `stop`, `start`, `try-restart`) but still run
`enable`/`disable`, which only edit unit symlinks. Remaining failures stay
visible but do not fail the package transaction. If an upgrade restart fails,
inspect the service logs and restart it manually after resolving the error.
