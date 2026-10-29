---
title: "Automatic rollback and terminal failures"
description: "How reconD rolls back a failed Docker deployment to the last known-good image, and how terminal failures prevent endless redeploy loops."
---

# Rollback and terminal failures

## Rollback

When a forward deployment fails after all of its
[attempts](/config/retries), reconD tries to restore the release recorded as
`current` in the [state file](./state), the last one that deployed
successfully.

A rollback runs the same steps as a deployment, with the previous release's
metadata:

1. Run the configured [strategy](/config/strategies).
2. Run `deploy_command`, with `WORKLOAD_*` describing the previous release.
3. Wait for the [health check](/config/health-checks).
4. Record the restored release as `current`, with `rollback_from` set to the
   failed digest.

A rollback is attempted up to `max_retry_rollback` times.

reconD keeps only one rollback generation. It can restore the last good
release, but not the one before it.

### When rollback is not possible

If the workload has never been deployed successfully, as on a fresh host,
there is nothing to roll back to. reconD sends a `failed` notification and
records a terminal failure.

Errors that are not deployment failures, such as being unable to read or write
the state file, abort the cycle without a rollback.

## Terminal failures

Once a release's forward attempts are exhausted, reconD records a terminal
failure for that workload and manifest digest:

```json
"terminal_failure": {
  "manifest_digest": "sha256:19779d90...",
  "git_sha": "10a3e42c...",
  "forward_deployment_attempts": 3,
  "rollback_attempts": 1,
  "reason": "deployment failed: ...; rollback succeeded",
  "failed_at": "2026-05-26T05:03:44Z"
}
```

This is recorded whether or not the rollback succeeds. It prevents a deploy
loop: without it, every cycle would see the bad release as drift, deploy it,
fail, and roll back again.

While the terminal failure matches the desired digest, each cycle logs a
warning and does nothing:

```text
level=WARN msg="automatic deployment skipped after retry exhaustion" digest=sha256:19779d90...
```

### Clearing a terminal failure

The terminal failure is cleared automatically when:

- **A newer release is published.** A different desired digest is not blocked.
  When it deploys successfully, the old terminal failure is removed. This is
  the normal fix: ship a corrected release.
- **The desired release is already running.** If the desired digest matches
  the recorded and running deployment, for example because the metadata was
  reverted to the current release, the terminal failure is removed.

To retry the same digest, for example after fixing a host-side problem such as
a missing env file or a full disk, remove the terminal failure by hand:

```sh
sudo systemctl stop recond
sudo cp /var/lib/recond/deployments_state.json /var/lib/recond/deployments_state.json.bak
sudo jq '(.deployments[] | select(.workload_name == "my-app")) |= del(.terminal_failure)' \
  /var/lib/recond/deployments_state.json.bak \
  | sudo tee /var/lib/recond/deployments_state.json >/dev/null
sudo systemctl start recond
```

The next cycle deploys the release again with a fresh set of attempts.
