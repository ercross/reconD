---
title: "Logging and journald queries"
description: "Configure reconD's structured JSON or text logs, understand log phases and fields, and query deployments with journalctl and jq."
---

# Logging

reconD writes structured logs to stdout. Under the packaged service, they go
to the journal:

```sh
sudo journalctl -u recond -f
sudo journalctl -u recond --since "1 hour ago"
```

## Format and level

```yaml
log:
  format: text   # default: JSON
  level: debug   # debug | info | warn | error; default: info
```

JSON, the default, suits journald and log shippers such as Promtail, Fluentd,
or the Datadog agent. `text` is easier to read during local development.

At `info`, a quiet workload logs almost nothing. Use `debug` to see every
cycle, including `no drift detected` and each `docker` command reconD runs.

## Fields

Every line from a workload's reconciler carries `workload` and `container`.
Slack delivery errors are the exception: they are logged without either. Most
lines also carry a `phase`, which you can filter or alert on:

| `phase` | Meaning |
| --- | --- |
| `poll` | Fetching deployment metadata from GitHub. |
| `drift_check` | Comparing desired, recorded, and running digests. |
| `runtime_inspect` | Running `docker inspect` on `container_name` (logged at `debug`). |
| `runtime_image_inspect` | Running `docker image inspect` to find the running image's digest (logged at `debug`). |
| `image_pull` | The `starting deployment` line. reconD does not pull images itself; your deploy command does. |
| `deployment_strategy` | Applying the [strategy](/config/strategies). |
| `restart` | Each forward attempt and its `deploy_command`. |
| `health_check` | Waiting for the [health check](/config/health-checks). |
| `commit` | Writing state, and prune failures. |
| `image_prune` | Running `docker image prune` after a successful deploy. |
| `rollback` | Restoring the previous release, including backoff between rollback attempts. |
| `forward_deployment` | Backoff before retrying a forward attempt. |
| `noop` | No drift; nothing to do. |

Other common fields are `digest`, `git_sha`, `attempt`, `max_attempts`,
`drift_reason`, `duration_ms`, and `error`. When `deploy_command` fails, its
`stdout` and `stderr` are included.

## Useful queries

With the default JSON format, `jq` can filter the journal. `fromjson?` skips
any lines that are not JSON:

```sh
# Everything one workload did
sudo journalctl -u recond -o cat | jq -R 'fromjson? | select(.workload == "my-app")'

# Only errors
sudo journalctl -u recond -o cat | jq -R 'fromjson? | select(.level == "ERROR")'

# Why reconD decided to deploy
sudo journalctl -u recond -o cat | jq -R 'fromjson? | select(.drift_reason) | {time, workload, drift_reason}'
```
