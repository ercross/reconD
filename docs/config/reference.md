---
title: "Configuration field reference"
description: "Every reconD config field with its default value and validation rules: workloads, GitHub provider, strategies, health checks, retries, and labels."
---

# Field reference

Every config field, with its default and validation rules.

## Top level

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `log` | No | | Process logging. See [`log`](#log). |
| `deployment_retry` | No | | Retry defaults for every workload. See [`deployment_retry`](#deployment-retry). |
| `workloads` | Yes | | List of workloads. At least one is required. |

## `log`

| Field | Default | Description |
| --- | --- | --- |
| `log.format` | JSON | `text` for human-readable output; any other value uses JSON. |
| `log.level` | `info` | `debug`, `info`, `warn`, or `error`. Unknown values use `info`. |

See [Logging](/operations/logging).

## Workload fields

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | Yes | | Identifies the workload in logs, notifications, and state. Must be unique. |
| `container_name` | Yes | | Docker container reconD inspects to find the running image digest. |
| `deploy_command` | Yes | | Shell command that deploys the workload. Run with `sh -c`. See [Deploy command](./deploy-command). |
| `git_provider.owner` | Yes | | GitHub user or organization that owns the metadata repository. |
| `git_provider.repo` | Yes | | GitHub repository whose releases carry `deployment-metadata.json`. |
| `git_provider.token` | No | `$GITHUB_TOKEN` | GitHub token. Needed for private repositories, and in practice for public ones because of GitHub's rate limit. |
| `release_prefix` | No | empty (any tag) | Only consider releases whose tag starts with this. See [Release prefixes](/guide/deployment-metadata#release-prefixes). |
| `check_interval` | No | `60s` | Time between reconciliation cycles. Values below `10s` are replaced with `60s`. |
| `notification_url` | No | disabled | Slack incoming webhook URL. See [Notifications](./notifications). |
| `strategy` | No | none | Preparation step before `deploy_command`. See [`strategy`](#strategy). |
| `health_check` | No | none | Post-deploy health check. See [`health_check`](#health-check). |
| `deployment_retry` | No | top-level values | Per-workload retry overrides. See [`deployment_retry`](#deployment-retry). |
| `labels` | No | none | Docker labels that scope image pruning after a successful deploy. See [Image pruning](#image-pruning). |

## `strategy`

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `strategy.type` | No | `none` | `none` or `env_file`. |
| `strategy.env_file_path` | With `env_file` | | Env file to create or update. Use an absolute path. |
| `strategy.image_tag_key` | No | `IMAGE_TAG` | Key written with the release's image tag. |

See [Deployment strategies](./strategies).

## `health_check`

| Field | Required | Default | Description |
| --- | --- | --- | --- |
| `health_check.type` | No | `none` | `none`, `http`, or `tcp`. `command` is recognized but rejected as not implemented. |
| `health_check.url` | With `http`/`tcp` | | `http://...` URL, or `tcp://host:port`. |
| `health_check.retries` | No | `12` | Total attempts before the deployment counts as unhealthy. |
| `health_check.interval` | No | `10s` | Wait between attempts. |
| `health_check.timeout` | No | `5s` | Timeout for each attempt. |

See [Health checks](./health-checks).

## `deployment_retry`

Can be set at the top level, per workload, or both. A workload's values
override only the fields they set.

| Field | Default | Rule | Description |
| --- | --- | --- | --- |
| `max_retry_forward_deployment` | `1` | ≥ 1 | Total forward deployment attempts before rollback. |
| `max_retry_rollback` | `1` | ≥ 1 | Total rollback attempts before giving up. |
| `initial_interval` | `1s` | > 0 | First backoff delay. |
| `max_interval` | `30s` | ≥ `initial_interval` | Cap on backoff delays. |
| `multiplier` | `2` | finite, ≥ 1 | Exponential backoff growth factor. |

See [Retries](./retries).

## Image pruning

After every successful forward deployment, reconD runs:

```sh
docker image prune -a --force --filter label=<key>=<value> ...
```

with one filter per entry in `labels`. This removes images that no container
uses and that carry all of the given labels. A prune failure is logged but does
not fail the deployment.

::: danger Set labels on every workload
If `labels` is empty, the command has no filter and removes **every** image on
the host that no container is currently using, including images other tools
rely on. Set `labels` to labels your images actually carry, for example with
`LABEL app=my-app` in the Dockerfile.
:::
