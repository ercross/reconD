---
title: "Upgrade, downgrade, and uninstall reconD"
description: "Upgrade or downgrade reconD with apt, dnf, or zypper, and uninstall it while keeping or removing its config and deployment state."
---

# Upgrade and uninstall

## Upgrade

Download the newer package and install it with the same command used for the
first install:

::: code-group

```sh [Debian / Ubuntu]
sudo apt install "./recond_${VERSION}_${ARCH}.deb"
```

```sh [Fedora / RHEL-compatible]
sudo dnf install "./recond-${VERSION}-1.${ARCH}.rpm"
```

```sh [openSUSE]
sudo zypper install --allow-unsigned-rpm "./recond-${VERSION}-1.${ARCH}.rpm"
```

:::

If the service is running, the package restarts it on the new binary. If it is
stopped, it stays stopped, and whether it starts on boot does not change. Your
config and state are left untouched.

```sh
recond -version
sudo systemctl status recond
```

If the restart after an upgrade fails, check `journalctl -u recond`, fix the
cause, and run `sudo systemctl restart recond`.

Read the [changelog](/changelog) before upgrading across a release with
breaking changes. v0.5.0, for example, moved the default config and state
paths.

## Downgrade

Download the older package and install it as a downgrade. The service is
restarted the same way as on an upgrade:

::: code-group

```sh [Debian / Ubuntu]
sudo apt install --allow-downgrades "./recond_${VERSION}_${ARCH}.deb"
```

```sh [Fedora / RHEL-compatible]
sudo dnf downgrade "./recond-${VERSION}-1.${ARCH}.rpm"
```

```sh [openSUSE]
sudo zypper install --oldpackage --allow-unsigned-rpm "./recond-${VERSION}-1.${ARCH}.rpm"
```

:::

## Uninstall

::: code-group

```sh [Debian / Ubuntu]
sudo apt remove recond   # keeps your config and deployment state
sudo apt purge recond    # removes everything, including /etc/recond and /var/lib/recond
```

```sh [Fedora / RHEL-compatible]
sudo dnf remove recond
```

```sh [openSUSE]
sudo zypper remove recond
```

:::

Uninstalling stops and disables the service.

::: warning apt purge deletes your config and state
On Debian and Ubuntu, `apt purge` also deletes `/etc/recond` (including your
`config.yaml`) and `/var/lib/recond` (including deployment state). Back them up
first if you may reinstall later. `apt remove`, `dnf remove`, and
`zypper remove` keep them.
:::

RPM has no purge, so remove the config and state yourself if you no longer
need them:

```sh
sudo rm -rf /etc/recond /var/lib/recond
```

## Reinstall after uninstall

Reinstalling reuses the kept config and state, but the service stays disabled
and stopped, because uninstalling disabled it. Start it again with:

```sh
sudo systemctl enable --now recond
```
