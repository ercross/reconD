---
title: "Getting started: auto-deploy Docker containers with reconD"
description: "Install reconD on a Linux server, configure a workload, and automatically deploy every new image your CI publishes to GitHub Releases."
---

# Getting started

This walkthrough takes a Linux host from nothing to a workload that reconD
keeps up to date. It assumes a Debian or Ubuntu host on `amd64`; the
[installation page](/install/) covers the other distributions and
architectures.

## Prerequisites

- A Linux host with systemd.
- Docker installed and running. Check that `sudo docker version` reaches the
  daemon.
- A GitHub repository whose releases carry a
  [`deployment-metadata.json`](./deployment-metadata) asset.
- A command that deploys your workload on the host, for example
  `/opt/my-app/deploy.sh`.

## 1. Install the package

```sh
VERSION=$(curl -fsSL https://api.github.com/repos/ercross/reconD/releases/latest | sed -n 's/.*"tag_name": *"v\([^"]*\)".*/\1/p')   # latest release
ARCH=amd64
curl -fsSLO "https://github.com/ercross/reconD/releases/download/v${VERSION}/recond_${VERSION}_${ARCH}.deb"
curl -fsSLO "https://github.com/ercross/reconD/releases/download/v${VERSION}/checksums.txt"
sha256sum --ignore-missing -c checksums.txt
sudo apt install "./recond_${VERSION}_${ARCH}.deb"
recond -version
```

The package installs the binary and a systemd unit, but it does not start the
service yet.

## 2. Write a config

Start from the example the package ships:

```sh
sudo cp /etc/recond/config.yaml.example /etc/recond/config.yaml
sudo chmod 600 /etc/recond/config.yaml
sudoedit /etc/recond/config.yaml
```

A minimal workload needs a name, the container to watch, the deploy command,
the repository to read releases from, and the labels that scope image pruning:

```yaml
workloads:
  - name: my-app
    container_name: my-app
    deploy_command: /opt/my-app/deploy.sh
    git_provider:
      owner: my-github-owner
      repo: my-app
      token: ghp_xxx   # needed in practice: unauthenticated requests are rate-limited
    health_check:
      type: http
      url: http://localhost:8080/health
    labels:
      app: my-app   # must match a LABEL on your image; scopes image pruning
```

Use absolute paths in `deploy_command`. The service runs it from `/`; see
[Deploy command](/config/deploy-command).

Set `labels` to labels your image carries, for example with `LABEL app=my-app`
in the Dockerfile. After each successful deploy, reconD prunes unused images
that carry these labels. Without `labels`, the prune removes every unused image
on the host; see [Image pruning](/config/reference#image-pruning).

## 3. Start the service

```sh
sudo systemctl enable --now recond
sudo journalctl -u recond -f
```

On its first cycle reconD has no recorded state, so it treats the workload as
drifted. It runs your deploy command with the release's image in
[`WORKLOAD_*` environment variables](/reference/deploy-command-env), waits for
the health check, and records the deployment in
`/var/lib/recond/deployments_state.json`.

## 4. Release a new version

Publish a new GitHub Release with an updated `deployment-metadata.json`. Within
one `check_interval` (60 seconds by default), reconD sees the new digest and
deploys it. If the health check fails, it rolls back to the previous version.

## Next steps

- [How it works](./how-it-works): what happens in each reconciliation cycle.
- [Configuration](/config/): every option, with defaults.
- [Running the service](/install/service): day-to-day service management.
