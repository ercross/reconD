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
workloads:
  - name: my-app
    release_prefix: my-app-prod
    container_name: my-container
    deploy_command: make redeploy-app
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

## State Files

State for all workloads is stored in one deployments state file. Set it with
the `-state` flag; it defaults to `/var/lib/reconD/deployments_state.json`.

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
the binary on the host, run it under a process manager such as systemd, keep
configuration under a host path such as `/etc/reconD`, and keep workload state
under a persistent host path such as `/var/lib/reconD`.

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
- `deploy_command`
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

## Design Rule

`reconD` ensures a single named workload is running the image specified by its
deployment metadata.

Everything else belongs to the deploy command or to external tooling.
