---
title: "Health checks and automatic rollback"
description: "Configure HTTP and TCP health checks that reconD waits on after each Docker deployment, and that trigger automatic rollback when they fail."
---

# Health checks

After `deploy_command` succeeds, reconD can wait for the workload to report
healthy before recording the deployment. A failed health check counts as a
deployment failure, which leads to a [retry or rollback](./retries).

Without a health check, reconD records the deployment as soon as the deploy
command exits with status `0`.

::: tip Configure a health check for automatic rollback
Without a health check, reconD rolls back only when the deploy command itself
fails. A release that deploys cleanly but then crashes or serves errors is
kept. Configure a health check to catch those too.
:::

| `health_check.type` | Success means |
| --- | --- |
| `none` (default) | No check. |
| `http` | `GET <url>` returns status `200` within `timeout`. |
| `tcp` | A TCP connection to the host and port in `url` opens within `timeout`. |
| `command` | Recognized but not implemented. Configs using it are rejected. |

## HTTP

```yaml
health_check:
  type: http
  url: http://localhost:8080/health
  retries: 12
  interval: 10s
  timeout: 5s
```

- Only status `200` counts as healthy. Other `2xx` codes do not.
- Redirects are not followed; a `301` or `302` counts as unhealthy.

## TCP

```yaml
health_check:
  type: tcp
  url: tcp://localhost:5432
```

The URL must include a host and port.

## Timing

reconD makes up to `retries` attempts, waiting `interval` between them, and
stops at the first success. With the defaults (12 attempts, 10s apart, 5s
timeout), a workload has between about 110 seconds (when each check fails
fast, for example with connection refused) and 170 seconds (when every check
times out) to become healthy before the attempt counts as failed.

The same check runs after a rollback. A rollback that fails its health check
counts as a failed rollback attempt.
