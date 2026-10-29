---
title: "Troubleshooting reconD deployments"
description: "Fix common reconD problems: restart loops, missing release metadata, GitHub rate limits, failing deploy commands, health checks, and redeploy loops."
---

# Troubleshooting

Start with the logs:

```sh
sudo systemctl status recond
sudo journalctl -u recond -n 200 --no-pager
```

Set `log.level: debug` and restart for more detail.

## The service restarts every 5 seconds

The config is missing or invalid. `journalctl` shows `load config error` with
the reason. Stop the loop, fix the config, and start again:

```sh
sudo systemctl stop recond
sudoedit /etc/recond/config.yaml
sudo systemctl start recond
```

## `no deployment metadata found`

None of the 30 newest releases in the repository has both a matching tag and a
`deployment-metadata.json` asset. Check that:

- `git_provider.owner` and `git_provider.repo` are correct.
- `release_prefix` matches the start of your release tags exactly.
- The asset is named exactly `deployment-metadata.json`.
- The release is not buried under 30 or more newer releases from other
  streams.

## `failed to fetch desired state`

- **`401` or `404` from GitHub** on a private repository: the token is missing,
  expired, or lacks read access to the repository.
- **`403` rate limit**: you are using unauthenticated requests. Set a token.
- **`parse metadata JSON`**: the asset is not valid JSON.
- **`deployment metadata image is required`** or
  **`manifest_digest is required`**: the metadata is missing a required
  field. See [Deployment metadata](/guide/deployment-metadata#format).

## The deploy command works by hand but fails under the service

Under systemd the command runs from `/` with `HOME=/root` and systemd's
`PATH`. Typical causes:

- A relative path such as `./deploy.sh`, `make`, or `docker compose` without
  `-f`. Use absolute paths or start with `cd /opt/my-app &&`.
- A tool installed only in your user's `PATH`, such as `~/.local/bin` or
  `/snap/bin`.
- Registry credentials stored for your user rather than root. Run
  `sudo docker login`.

The failing command's `stdout` and `stderr` are in the `deploy command failed`
log line.

## reconD does not redeploy after I stop the container

This is by design. A stopped or missing container counts as no drift; reconD
does not manage container lifecycle. Use a Docker restart policy, or restart it
by hand.

## A release is never deployed: `automatic deployment skipped after retry exhaustion`

That digest exhausted its attempts earlier and is recorded as a terminal
failure. Publish a fixed release, or
[clear the terminal failure](./rollback#clearing-a-terminal-failure) to retry
the same one.

## reconD keeps redeploying the same release

Check the `drift_reason` in the logs:

- **`runtime container manifest digest is unavailable`**: the container's image
  has no repository digest, which usually means it was built locally rather
  than pulled from a registry. Deploy images pulled from a registry.
- **`current state digest [...] != runtime container digest [...]`**: the
  container named `container_name` runs a different image from the one in the
  metadata. Check that your deploy command runs the image the metadata names,
  and that `container_name` is the container it starts.

## Health check fails although the app is up

- HTTP checks require exactly status `200` and do not follow redirects.
- The app may need longer to start than `retries × interval`. Increase
  either.
- `localhost` refers to the host. Make sure the container publishes the port.

## Images other tools need were deleted

After a successful deploy, reconD prunes unused images. With empty `labels`
this prune is host-wide. Set `labels` on every workload; see
[Image pruning](/config/reference#image-pruning).
