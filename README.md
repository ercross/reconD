# reconD

**reconD** is a lightweight, single-host GitOps-style continuous deployment agent written in Go. It continuously reconciles desired container image states with observed host states, bringing Kubernetes-style reconciliation loops to standalone Docker environments.

It watches deployment metadata that your CI publishes on GitHub Releases,
compares the desired image digest with the host's observed state, and runs one
configured deploy command when drift is found. With a health check
configured, it waits for the new release to become healthy, and rolls back to
the last known-good release if it does not.

The project is intentionally narrow: it is a reconciliation loop for workload
image state. It is not a container orchestrator.

**Documentation: <https://ercross.github.io/reconD/>**

## Installation

`reconD` ships as `.deb` and `.rpm` packages for `amd64`/`x86_64` and
`arm64`/`aarch64` on [GitHub Releases](https://github.com/ercross/reconD/releases).
It needs Docker on the host.

```sh
VERSION=$(curl -fsSL https://api.github.com/repos/ercross/reconD/releases/latest | sed -n 's/.*"tag_name": *"v\([^"]*\)".*/\1/p')   # latest release
ARCH=amd64   # or arm64
curl -fsSLO "https://github.com/ercross/reconD/releases/download/v${VERSION}/recond_${VERSION}_${ARCH}.deb"
curl -fsSLO "https://github.com/ercross/reconD/releases/download/v${VERSION}/checksums.txt"
sha256sum --ignore-missing -c checksums.txt
sudo apt install "./recond_${VERSION}_${ARCH}.deb"
```

See [Installation](https://ercross.github.io/reconD/install/) for Fedora,
RHEL-compatible, and openSUSE hosts, upgrades, and building from source.

## Configure and run

```sh
sudo cp /etc/recond/config.yaml.example /etc/recond/config.yaml
sudo chmod 600 /etc/recond/config.yaml
sudoedit /etc/recond/config.yaml
sudo systemctl enable --now recond
```

```yaml
workloads:
  - name: my-app
    release_prefix: my-app-prod
    container_name: my-app
    deploy_command: cd /opt/my-app && make redeploy-app
    git_provider:
      owner: my-github-owner
      repo: my-app
    health_check:
      type: http
      url: http://localhost:8080/health
    labels:
      app: my-app   # must match a LABEL on your image; scopes image pruning
```

## Documentation

| Section | Covers |
| --- | --- |
| [Guide](https://ercross.github.io/reconD/guide/what-is-recond) | What reconD is for, a walkthrough, the reconciliation loop, and the deployment metadata format. |
| [Installation](https://ercross.github.io/reconD/install/) | Packages, the systemd service, upgrades, and uninstalling. |
| [Configuration](https://ercross.github.io/reconD/config/) | Every config field, deploy commands, strategies, health checks, retries, and notifications. |
| [Operations](https://ercross.github.io/reconD/operations/state) | State, rollback and terminal failures, logging, security, and troubleshooting. |
| [Contributing](https://ercross.github.io/reconD/contributing/development) | Building, testing, packaging, and releasing. |

The documentation source is in [`docs/`](docs/). See the
[changelog](CHANGELOG.md) for release notes.

## License

[MIT](LICENSE)
