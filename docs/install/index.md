---
title: "Install reconD on Debian, Ubuntu, Fedora, RHEL, and openSUSE"
description: "Install reconD from .deb and .rpm packages for amd64 and arm64, verify checksums, set up Docker, and create your first config."
---

# Linux packages

reconD is published as `.deb` and `.rpm` packages on
[GitHub Releases](https://github.com/ercross/reconD/releases). The package and
the installed binary are both named `recond`.

| Distribution family | Architecture | Package file |
| --- | --- | --- |
| Debian, Ubuntu | `amd64` | `recond_<version>_amd64.deb` |
| Debian, Ubuntu | `arm64` | `recond_<version>_arm64.deb` |
| Fedora, RHEL-compatible, openSUSE | `x86_64` | `recond-<version>-1.x86_64.rpm` |
| Fedora, RHEL-compatible, openSUSE | `aarch64` | `recond-<version>-1.aarch64.rpm` |

Each release also includes `checksums.txt` with SHA-256 checksums for every
artifact.

The commands below look up the latest release with the GitHub API. To install
a specific version instead, set it directly, for example `VERSION=0.5.0`.

## Install Docker first

reconD needs the `docker` CLI and a running Docker daemon. Install Docker from
your distribution or from
[Docker's repositories](https://docs.docker.com/engine/install/), then check
that `sudo docker version` reaches the daemon.

The packages only *recommend* Docker, because its package name differs across
distributions. If Docker is not installed, the package manager installs the
recommendation itself:

- `apt` installs the distribution's `docker.io` package.
- `dnf` installs whichever available package provides `/usr/bin/docker`.
  RHEL-compatible distributions do not ship Docker, so there it installs
  `podman-docker`, a Podman compatibility wrapper. reconD is not tested against
  Podman. `podman-docker` also blocks a later Docker install, because Docker's
  `docker-ce-cli` installs the same `/usr/bin/docker` file; run
  `sudo dnf remove podman-docker` before installing Docker.
- `zypper` installs openSUSE's `docker` package.

To skip the recommendation, add `--no-install-recommends` to `apt install`,
`--setopt=install_weak_deps=False` to `dnf install`, or `--no-recommends` to
`zypper install`.

::: danger Snap Docker is not supported
Docker installed as a snap (`snap install docker`) is not supported. `apt` does
not see the snap, so it installs `docker.io` next to it, and the two daemons
compete for `/var/run/docker.sock`. With `--no-install-recommends` instead, the
service still cannot find `docker`, because systemd's `PATH` does not include
`/snap/bin`. Remove the snap and install Docker from the distribution or from
Docker's repositories.
:::

## Install

::: code-group

```sh [Debian / Ubuntu]
VERSION=$(curl -fsSL https://api.github.com/repos/ercross/reconD/releases/latest | sed -n 's/.*"tag_name": *"v\([^"]*\)".*/\1/p')   # latest release
ARCH=amd64   # or arm64
curl -fsSLO "https://github.com/ercross/reconD/releases/download/v${VERSION}/recond_${VERSION}_${ARCH}.deb"
curl -fsSLO "https://github.com/ercross/reconD/releases/download/v${VERSION}/checksums.txt"
sha256sum --ignore-missing -c checksums.txt
sudo apt install "./recond_${VERSION}_${ARCH}.deb"
```

```sh [Fedora / RHEL-compatible]
VERSION=$(curl -fsSL https://api.github.com/repos/ercross/reconD/releases/latest | sed -n 's/.*"tag_name": *"v\([^"]*\)".*/\1/p')   # latest release
ARCH=x86_64   # or aarch64
curl -fsSLO "https://github.com/ercross/reconD/releases/download/v${VERSION}/recond-${VERSION}-1.${ARCH}.rpm"
curl -fsSLO "https://github.com/ercross/reconD/releases/download/v${VERSION}/checksums.txt"
sha256sum --ignore-missing -c checksums.txt
sudo dnf install "./recond-${VERSION}-1.${ARCH}.rpm"
```

```sh [openSUSE]
VERSION=$(curl -fsSL https://api.github.com/repos/ercross/reconD/releases/latest | sed -n 's/.*"tag_name": *"v\([^"]*\)".*/\1/p')   # latest release
ARCH=x86_64   # or aarch64
curl -fsSLO "https://github.com/ercross/reconD/releases/download/v${VERSION}/recond-${VERSION}-1.${ARCH}.rpm"
curl -fsSLO "https://github.com/ercross/reconD/releases/download/v${VERSION}/checksums.txt"
sha256sum --ignore-missing -c checksums.txt
# Packages are not signed yet
sudo zypper install --allow-unsigned-rpm "./recond-${VERSION}-1.${ARCH}.rpm"
```

:::

Check the installed version:

```sh
recond -version
```

## Installed files

| Path | Purpose |
| --- | --- |
| `/usr/bin/recond` | Agent binary |
| `/usr/lib/systemd/system/recond.service` | systemd unit |
| `/etc/recond/config.yaml.example` | Example config shipped by the package |
| `/etc/recond/config.yaml` | Your config. You create it; packages never overwrite it. |
| `/var/lib/recond/deployments_state.json` | Deployment state, created by the agent |
| `/usr/share/doc/recond/copyright` (`.deb`), `/usr/share/licenses/recond/LICENSE` (`.rpm`) | License |

## Configure

The package does not enable or start the service on a fresh install. Create a
config first:

```sh
sudo cp /etc/recond/config.yaml.example /etc/recond/config.yaml
sudo chmod 600 /etc/recond/config.yaml
sudoedit /etc/recond/config.yaml
```

Keep the file mode at `600` if you put a GitHub token in it. See
[Configuration](/config/) for every field, then
[start the service](./service).
