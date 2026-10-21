# reconD

`reconD` is a small deployment agent for single-host container workloads.

It watches deployment metadata, compares the desired image digest with the
host's observed state, and runs one configured deployment command when drift is
found.

The project is intentionally narrow: it is a reconciliation loop for workload
image state. It is not a container orchestrator.

## What It Does

For each configured workload, `reconD` repeatedly:

1. Fetches the latest relevant `deployment-metadata.json`.
2. Reads the currently committed deployment state from disk.
3. Inspects the configured container to detect runtime drift.
4. Pulls the desired image.
5. Runs an optional deployment preparation strategy.
6. Runs the configured `deploy_command`.
7. Waits for the health check.
8. Commits deployment state, or attempts rollback on failure.

The desired state is the deployment metadata emitted alongside an image release.
The container registry is not treated as the source of truth by itself.

## Specific Use Case

Use `reconD` when you already have:

- CI that builds and publishes container images.
- CI that publishes `deployment-metadata.json` alongside image releases.
- A single host that should run one named workload at a desired image version.
- A local command that knows how to deploy that workload on the host.

The deploy command can be `docker run`, a shell script, a Make target, or a
command that invokes Docker Compose. `reconD` does not understand or manage the
topology behind that command.

## Out Of Scope

`reconD` does not manage:

- Docker Compose projects.
- Multi-container application topology.
- Service dependencies.
- Networking.
- Volumes.
- Secrets.
- Ingress or traffic routing.
- Load balancing.
- Scaling.
- Scheduling.
- Clusters.
- Infrastructure provisioning.

If a feature requires understanding application topology, dependency ordering,
networking, routing, or workload composition, it belongs outside the agent.

## Installation

`reconD` is published as `.deb` and `.rpm` packages on
[GitHub Releases](https://github.com/ercross/reconD/releases). The package and
the installed binary are both named `recond`.

| Distribution family                     | Architecture       | Package file                       |
| --------------------------------------- | ------------------ | ---------------------------------- |
| Debian, Ubuntu                          | `amd64`            | `recond_<version>_amd64.deb`       |
| Debian, Ubuntu                          | `arm64`            | `recond_<version>_arm64.deb`       |
| Fedora, RHEL-compatible, openSUSE       | `x86_64`           | `recond-<version>-1.x86_64.rpm`    |
| Fedora, RHEL-compatible, openSUSE       | `aarch64`          | `recond-<version>-1.aarch64.rpm`   |

Each release also includes `checksums.txt` with SHA-256 checksums for every
artifact.

`reconD` needs the `docker` CLI and a running Docker daemon. The packages only
recommend Docker, because its package name differs across distributions.
Install Docker first, from your distribution or from
[Docker's repositories](https://docs.docker.com/engine/install/), and check that
`sudo docker version` reaches the daemon.

If Docker is not installed, the package manager installs the recommendation
itself and picks the package:

- `apt` installs the distribution's `docker.io` package.
- `dnf` installs whichever available package provides `/usr/bin/docker`.
  RHEL-compatible distributions do not ship Docker, so there it installs
  `podman-docker`, a Podman compatibility wrapper. `reconD` is not tested
  against Podman. `podman-docker` also blocks a later Docker install, because
  Docker's `docker-ce-cli` installs the same `/usr/bin/docker` file; run
  `sudo dnf remove podman-docker` before installing Docker.
- `zypper` installs openSUSE's `docker` package.

To install `reconD` without the recommendation, add `--no-install-recommends`
to `apt install`, `--setopt=install_weak_deps=False` to `dnf install`, or
`--no-recommends` to `zypper install`.

Docker installed as a snap (`snap install docker`) is not supported. `apt`
does not see the snap, so it installs `docker.io` next to it, and the two
daemons compete for `/var/run/docker.sock`. With `--no-install-recommends`
instead, the service still cannot find `docker`, because systemd's `PATH`
does not include `/snap/bin`. Remove the snap and install Docker from the
distribution or from Docker's repositories.

### Install on Debian or Ubuntu

```sh
VERSION=0.5.0
ARCH=amd64   # or arm64
curl -fsSLO "https://github.com/ercross/reconD/releases/download/v${VERSION}/recond_${VERSION}_${ARCH}.deb"
curl -fsSLO "https://github.com/ercross/reconD/releases/download/v${VERSION}/checksums.txt"
sha256sum --ignore-missing -c checksums.txt
sudo apt install "./recond_${VERSION}_${ARCH}.deb"
```

### Install on Fedora, RHEL-compatible, or openSUSE

```sh
VERSION=0.5.0
ARCH=x86_64   # or aarch64
curl -fsSLO "https://github.com/ercross/reconD/releases/download/v${VERSION}/recond-${VERSION}-1.${ARCH}.rpm"
curl -fsSLO "https://github.com/ercross/reconD/releases/download/v${VERSION}/checksums.txt"
sha256sum --ignore-missing -c checksums.txt

# Fedora / RHEL-compatible
sudo dnf install "./recond-${VERSION}-1.${ARCH}.rpm"

# openSUSE (packages are not signed yet)
sudo zypper install --allow-unsigned-rpm "./recond-${VERSION}-1.${ARCH}.rpm"
```

Check the installed version:

```sh
recond -version
```

### Installed files

| Path                                       | Purpose                                         |
| ------------------------------------------ | ----------------------------------------------- |
| `/usr/bin/recond`                          | Agent binary                                    |
| `/usr/lib/systemd/system/recond.service`   | systemd unit                                    |
| `/etc/recond/config.yaml.example`          | Example config shipped by the package           |
| `/etc/recond/config.yaml`                  | Your config (you create this; never overwritten) |
| `/var/lib/recond/deployments_state.json`   | Deployment state (created by the agent)         |
| `/usr/share/doc/recond/copyright` (`.deb`), `/usr/share/licenses/recond/LICENSE` (`.rpm`) | License |

### Configure

The package does not enable or start the service on a fresh install. Create a
config first:

```sh
sudo cp /etc/recond/config.yaml.example /etc/recond/config.yaml
sudo chmod 600 /etc/recond/config.yaml
sudoedit /etc/recond/config.yaml
```

Set `git_provider.token` in the config if the release metadata repository is
private. Keep the file mode at `600`, because it then holds a credential. See
[Configuration](#configuration) for every field.

### Run the service

```sh
sudo systemctl enable --now recond   # start now and on boot
sudo systemctl status recond
sudo journalctl -u recond -f         # follow logs
sudo systemctl stop recond
sudo systemctl restart recond        # after editing the config
sudo systemctl disable recond        # do not start on boot
```

Create `/etc/recond/config.yaml` before starting the service. If the config
file is missing or invalid, `reconD` exits with an error and systemd restarts
it every 5 seconds, indefinitely, logging the same `load config error` each
time. To stop the loop, run `sudo systemctl stop recond`, fix the config, then
start it again. `journalctl -u recond` shows the exact error.

The service runs as `root`. `reconD` drives the host Docker daemon and runs
your `deploy_command`, and access to the Docker daemon is equivalent to root
access on the host, so a dedicated unprivileged user would not meaningfully
reduce what the agent can do. Only put commands you trust in `deploy_command`,
and keep `/etc/recond/config.yaml` writable by root only.

Under the service, `deploy_command` runs from `/`, not from the directory you
ran `reconD` from when testing by hand. `HOME` is `/root`. Use absolute paths in
`deploy_command` and `strategy.env_file_path`, or start the command with `cd`,
for example `cd /opt/my-app && make redeploy-app`. A relative `./deploy.sh`,
`make` target, or Compose file would otherwise be looked up in `/`.

### Upgrade

Download the newer package and install it with the same command used for the
first install:

```sh
sudo apt install "./recond_${VERSION}_${ARCH}.deb"                              # Debian / Ubuntu
sudo dnf install "./recond-${VERSION}-1.${ARCH}.rpm"                            # Fedora / RHEL-compatible
sudo zypper install --allow-unsigned-rpm "./recond-${VERSION}-1.${ARCH}.rpm"    # openSUSE
```

If the service is running, the package restarts it on the new binary. If it is
stopped, it stays stopped, and whether it starts on boot does not change. Your
config and state are left untouched.

```sh
recond -version
sudo systemctl status recond
```

If the restart after an upgrade fails, check `journalctl -u recond`, fix the
cause, and run `sudo systemctl restart recond`.

To go back to an older version, download its package and install it as a
downgrade. The service is restarted the same way as on an upgrade:

```sh
sudo apt install --allow-downgrades "./recond_${VERSION}_${ARCH}.deb"           # Debian / Ubuntu
sudo dnf downgrade "./recond-${VERSION}-1.${ARCH}.rpm"                          # Fedora / RHEL-compatible
sudo zypper install --oldpackage --allow-unsigned-rpm "./recond-${VERSION}-1.${ARCH}.rpm"  # openSUSE
```

### Uninstall

```sh
# Debian / Ubuntu
sudo apt remove recond   # keeps your config and deployment state
sudo apt purge recond    # removes everything, including /etc/recond and /var/lib/recond

# Fedora / RHEL-compatible
sudo dnf remove recond

# openSUSE
sudo zypper remove recond
```

Uninstalling stops and disables the service. On Debian and Ubuntu, `apt purge`
also deletes `/etc/recond` (including your `config.yaml`) and `/var/lib/recond`
(including deployment state); back them up first if you may reinstall later.
`apt remove`, `dnf remove`, and `zypper remove` keep them.

Reinstalling after an uninstall reuses the kept config and state, but the
service stays disabled and stopped. Start it again with
`sudo systemctl enable --now recond`.

RPM has no purge, so remove the config and state yourself if you no longer
need them:

```sh
sudo rm -rf /etc/recond /var/lib/recond
```

### Build from source

Building from source remains supported for contributors and anyone who prefers
manual installs. Unlike the packages, nothing is installed, created, or
upgraded for you.

```sh
git clone https://github.com/ercross/reconD.git
cd reconD
go build -o recond .
sudo install -m 0755 recond /usr/bin/recond
sudo install -m 0644 packaging/systemd/recond.service /usr/lib/systemd/system/recond.service
sudo systemctl daemon-reload
```

Then create `/etc/recond/config.yaml` as described under [Configure](#configure).
To upgrade, pull, rebuild, reinstall the binary, and restart the service. A
source build reports its version as `dev`.

To build the release packages locally, install
[GoReleaser](https://goreleaser.com) and run `make release-test`. Packages are
written to `dist/`.

## Deployment Metadata

The Git provider implementation fetches and unmarshals `deployment-metadata.json`
from GitHub Releases.

By default, reconD checks releases newest-first and deploys the first release
that has a valid `deployment-metadata.json` asset. If a workload config sets
`release_prefix`, reconD only considers release tags that start with that
prefix. This is recommended when one repository publishes multiple deployable
artifacts, packages, or deployment streams.

For example, `release_prefix: api-prod` matches release tags such as
`api-prod-sha-10a3e42`, while `release_prefix: worker-staging` matches
`worker-staging-sha-8d1af01`.

Example metadata:

```json
{
  "image": "ghcr.io/my-account/app-repo",
  "image_tag": "prod-sha-10a3e42",
  "manifest_digest": "sha256:19779d908d890f704a2170d7dde679e266a9137613ea299b38939eb889545f8e",
  "git_sha": "10a3e42cc477e9e6da84037474d45c8b1300e9f8",
  "created_at": "2026-05-26T04:47:49Z"
}
```

## Configuration

`reconD` loads YAML config files. Config paths must use a `.yaml` or `.yml`
extension.

Every config file must define a top-level `workloads` list, even when it only
contains one workload.

Minimal example:

```yaml
deployment_retry:
  max_retry_forward_deployment: 3
  max_retry_rollback: 2
  initial_interval: 1s
  max_interval: 30s
  multiplier: 2

workloads:
  - name: my-app
    release_prefix: my-app-prod
    container_name: my-container
    deploy_command: cd /opt/my-app && make redeploy-app
    check_interval: 60s
    git_provider:
      owner: my-account
      repo: app-repo
    strategy:
      type: env_file
      env_file_path: /path/to/.env
      image_tag_key: IMAGE_TAG
    health_check:
      type: http
      url: http://localhost:8080/health
      retries: 12
      interval: 10s
      timeout: 5s
    labels:
      app: my-app
```

## Deployment Strategy

Strategies run after the image is pulled and before `deploy_command` is
executed.

The first supported strategy is `env_file`.

```yaml
strategy:
  type: env_file
  env_file_path: /opt/gymportal/.env.deploy
  image_tag_key: IMAGE_TAG
```

This updates or creates the env file and sets:

```text
IMAGE_TAG=<latest image tag>
```

If `image_tag_key` is omitted for the `env_file` strategy, it defaults to
`IMAGE_TAG`.

This is useful when the deploy command delegates to a local script or Compose
file that reads the image tag from an env file.

## Health Checks

Supported health check types:

- `http`
- `tcp`
- `none`

Recognized but not implemented:

- `command`

HTTP example:

```yaml
health_check:
  type: http
  url: http://localhost:8080/health
  retries: 12
  interval: 10s
  timeout: 5s
```

## Deployment Retries

By default, `reconD` keeps the original single-attempt behavior:

```yaml
deployment_retry:
  max_retry_forward_deployment: 1
  max_retry_rollback: 1
  initial_interval: 1s
  max_interval: 30s
  multiplier: 2
```

Increase `max_retry_forward_deployment` to retry transient strategy,
`deploy_command`, or health-check failures before rollback. Rollback starts only
after the forward deployment attempts are exhausted. Increase
`max_retry_rollback` to retry rollback failures. Retry waits use exponential
backoff with jitter and stop promptly when the agent context is canceled.

Set `deployment_retry` at the top level to define defaults for every workload in
the config. A workload can include its own `deployment_retry` section to
override only the fields that differ from the top-level defaults.

If forward deployment attempts are exhausted, `reconD` persists a terminal
failure for that workload and manifest digest in the central state file. When a
previous deployment exists, rollback still runs to recover the workload, but a
successful rollback does not make the failed desired digest eligible for
automatic retry. Future reconciliation cycles skip that same digest until the
terminal failure is cleared from state. A newer desired digest is not blocked.

## State Files

State for all workloads is stored in one deployments state file. Set it with
the `-state` flag; it defaults to `/var/lib/recond/deployments_state.json`.

File:

- `deployments_state.json`: the current and previous successful deployment for
  each workload, identified by workload name.

The state includes workload name, image, image tag, manifest digest, git SHA,
deployment time, and rollback information when applicable.

## Rollback

If deployment fails after there is a previous successful deployment, `reconD`
attempts rollback using the previous state.

Rollback runs the same deployment flow with previous metadata:

1. Pull previous image.
2. Run the configured strategy with previous metadata.
3. Run `deploy_command`.
4. Wait for health.
5. Commit rollback state.

Only one rollback generation is maintained initially.

## Running Locally

Prerequisites:

- Go installed.
- Docker installed and available on `PATH`.
- A GitHub token if the release metadata is private.
- A config file.

Run tests:

```sh
env GOCACHE=/private/tmp/recond-go-build go test ./...
```

Run the agent:

```sh
GITHUB_TOKEN=your_token_here go run . -config your-config.yaml
```

Logging is JSON at `info` level by default. Configure logging in your config
file when you want a different format or level:

```yaml
log:
  format: text
  level: debug
```

## Operating Model

For production use, `reconD` is best treated as a host-level service: install
it from the [packages](#installation), which run it under systemd, keep
configuration in `/etc/recond`, and keep workload state in `/var/lib/recond`.

This matches the agent's job. It observes host container state, runs the local
`docker` command, writes local deployment state, and executes the configured
`deploy_command`, which may depend on host paths, scripts, Make targets, Compose
files, or environment files.

Running `reconD` as a Docker container is possible, but it should be considered
an advanced packaging option rather than the default operating model. A
containerized agent typically needs access to the host Docker socket, persistent
state and config mounts, and sometimes host networking or host project
directories. Mounting the Docker socket gives the agent broad control over the
host Docker daemon, so the security boundary is not the same as an ordinary
isolated application container.

## Local Test Fixture

The `test/` directory contains a sample config and local Docker Compose fixture.

Example:

```sh
make -f ./test/Makefile dev-up
GITHUB_TOKEN=your_token_here go run . -config ./test/sample.config.yaml
```

The sample deploy command calls:

```sh
make -f ./test/Makefile dev-redeploy
```

That Make target is just the local deployment command. `reconD` does not parse
or manage the Compose file.

## Configuration Fields

Required workload fields:

- `name`
- `container_name`
- `deploy_command`: run with `sh -c`. Under the packaged service it runs from
  `/`; see [Run the service](#run-the-service).
- `git_provider.owner`
- `git_provider.repo`
- `strategy.type`

Common optional fields:

- `log.format`: log output format. Set to `text` for local development; JSON is the default.
- `log.level`: minimum log level. Supports `debug`, `info`, `warn`, and `error`; defaults to `info`.
- `release_prefix`: release tag prefix used to scope metadata lookup. Recommended when a repository publishes more than one deployable artifact stream.
- `notification_url`: Slack webhook URL.
- `check_interval`: defaults to `60s` when below `10s`.
- `health_check.retries`: defaults to `12`.
- `health_check.interval`: defaults to `10s`.
- `health_check.timeout`: defaults to `5s`.
- `labels`: labels used to scope Docker image pruning.
- `deployment_retry.max_retry_forward_deployment`: total forward deployment attempts before rollback; defaults to `1`.
- `deployment_retry.max_retry_rollback`: total rollback recovery attempts after forward deployment is exhausted; defaults to `1`.
- `deployment_retry.initial_interval`: first retry backoff delay; defaults to `1s`.
- `deployment_retry.max_interval`: maximum retry backoff delay; defaults to `30s`.
- `deployment_retry.multiplier`: exponential backoff multiplier; defaults to `2`.

## Design Rule

`reconD` ensures a single named workload is running the image specified by its
deployment metadata.

Everything else belongs to the deploy command or to external tooling.
