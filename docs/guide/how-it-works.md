---
title: "How reconD works: reconciliation loop and drift detection"
description: "How reconD detects drift by image manifest digest, runs your deploy command, waits for health checks, and rolls back failed Docker deployments."
---

# How it works

reconD starts one reconciler per configured workload. Each reconciler runs a
cycle immediately at startup, then once every `check_interval`. Workloads are
independent: a slow or failing deployment of one does not delay the others.

## The reconciliation cycle

```text
fetch desired ─► load recorded state ─► detect drift ─► deploy ─► health check ─► commit
                                              │                        │
                                              └─ no drift: done        └─ failure: retry, then roll back
```

For each workload, a cycle:

1. **Fetches the desired state.** reconD reads the newest GitHub Release whose
   tag matches the workload's `release_prefix` and that has a
   `deployment-metadata.json` asset. See
   [Deployment metadata](./deployment-metadata).
2. **Loads the recorded state.** It reads the workload's current deployment
   from the [state file](/operations/state).
3. **Detects drift** (described below). With no drift, the cycle ends here,
   and any terminal failure recorded for the workload is cleared.
4. **Checks for a terminal failure.** If there is drift but an earlier cycle
   gave up on this exact digest, the cycle stops here. See
   [Rollback and terminal failures](/operations/rollback).
5. **Applies the deployment strategy**, if one is configured. The `env_file`
   strategy writes the image tag into an env file. See
   [Deployment strategies](/config/strategies).
6. **Runs `deploy_command`** with `sh -c`, passing the release in
   [`WORKLOAD_*` environment variables](/reference/deploy-command-env).
7. **Waits for the health check**, if one is configured. See
   [Health checks](/config/health-checks).
8. **Commits the new state.** The new release becomes `current`, and the
   rollback target if the next deployment fails.
9. **Prunes unused images**, scoped by the workload's `labels`.

If any of steps 5 to 7 fails, reconD tries again, up to
`max_retry_forward_deployment` attempts in total, then rolls back to the
release that was running before. See [Retries](/config/retries).

::: info Image pulls
reconD does not run `docker pull` itself. Your deploy command is responsible
for obtaining the image, for example with `docker pull "$WORKLOAD_IMAGE:$WORKLOAD_IMAGE_TAG"`
or `docker compose pull`.
:::

## Drift detection

reconD identifies versions by **manifest digest** (`sha256:...`), never by tag,
because tags are mutable. A workload has drifted when any of these is true:

| Condition | Drift reason in logs |
| --- | --- |
| No deployment has been recorded for the workload | `no current deployment state` |
| The recorded digest differs from the desired digest | `current digest [...] != desired digest [...]` |
| The container is running, but reconD cannot determine its image digest | `runtime container manifest digest is unavailable` |
| The container is running a different digest from the recorded one | `current state digest [...] != runtime container digest [...]` |

The last check catches changes made outside reconD, such as someone running a
different image by hand.

To find the running digest, reconD runs `docker inspect <container_name>`. It
uses the digest pinned in the container's image reference (`image@sha256:...`)
when present, and otherwise the image's `RepoDigests`.

If the container does not exist or is stopped, reconD reports **no drift**.
Container lifecycle is out of scope; reconD does not restart stopped
containers.

## Concurrency and shutdown

All workloads share one state file. Writes are serialized and use an atomic
rename, so a reconD crash never leaves a half-written file. See
[Durability](/operations/state#durability) for what a power loss can do.

On `SIGTERM` or `SIGINT`, reconD cancels in-flight work: the running deploy
command or health check is stopped, retry backoffs end, and no rollback or
terminal failure is recorded for the interrupted attempt. The next start
re-evaluates drift from the recorded state.
