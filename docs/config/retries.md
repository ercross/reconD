---
title: "Deployment retries with exponential backoff"
description: "Configure how many times reconD retries a failed deployment or rollback, with exponential backoff and jitter, globally or per workload."
---

# Retries

By default, reconD makes one forward deployment attempt and one rollback
attempt. `deployment_retry` raises both limits and controls the delay between
attempts.

```yaml
deployment_retry:
  max_retry_forward_deployment: 3
  max_retry_rollback: 2
  initial_interval: 1s
  max_interval: 30s
  multiplier: 2
```

## What is retried

A forward attempt is retried when any of these fails:

- the [deployment strategy](./strategies)
- `deploy_command`
- the [health check](./health-checks)

Rollback starts only after all `max_retry_forward_deployment` attempts have
failed. Each rollback attempt runs the strategy, `deploy_command`, and health
check again with the previous release, up to `max_retry_rollback` times.

Other errors, such as failing to write the state file, are not retried.

## Backoff

The delay after failed attempt *n* starts at
`initial_interval × multiplier^(n-1)`, capped at `max_interval`, then gets
±25% random jitter (still capped at `max_interval`). With the example above,
the delays are about 1s, then 2s.

Stopping reconD interrupts a backoff immediately.

## Defaults and overrides

Set `deployment_retry` at the top level to give every workload the same
policy. A workload's own `deployment_retry` overrides only the fields it sets:

```yaml
deployment_retry:
  max_retry_forward_deployment: 3
  max_retry_rollback: 2

workloads:
  - name: api
    # uses 3 forward attempts, 2 rollback attempts
  - name: batch
    deployment_retry:
      max_retry_forward_deployment: 1   # 1 forward attempt, still 2 rollback attempts
```

## When retries run out

Once forward attempts are exhausted, reconD records a **terminal failure** for
that workload and digest, and stops trying to deploy that digest automatically,
whether or not rollback succeeds. A newer release is not blocked. See
[Rollback and terminal failures](/operations/rollback).
