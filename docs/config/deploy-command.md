---
title: "Deploy command: docker run, Docker Compose, or scripts"
description: "How reconD runs your deploy command, with examples for docker run, Docker Compose, and shell scripts that deploy a pinned image digest."
---

# Deploy command

`deploy_command` is the one thing reconD runs to change what is deployed. reconD
does not know what it does; it only checks the exit status and then the health
check.

```yaml
deploy_command: cd /opt/my-app && docker compose pull app && docker compose up -d app
```

## How it runs

- It runs with `sh -c "<deploy_command>"`, so pipes, `&&`, and variables work.
- It inherits reconD's environment plus the
  [`WORKLOAD_*` variables](/reference/deploy-command-env) describing the
  release being deployed.
- Exit status `0` means success. Anything else is a deployment failure that
  triggers a [retry or rollback](./retries).
- stdout and stderr are captured and logged only when the command fails.
- Under the packaged service, the working directory is `/` and `HOME` is
  `/root`. Use absolute paths or start with `cd`.
- The command has no timeout of its own. A command that hangs blocks that
  workload's reconciler until reconD is stopped.

The same command runs during [rollback](/operations/rollback), with
`WORKLOAD_*` describing the previous release instead.

## The command must fetch the image

reconD does not pull images. Your command must make sure the desired image is
present, for example by running `docker pull` or `docker compose pull`.

## Examples

### Plain `docker run`

```yaml
deploy_command: >-
  docker pull "$WORKLOAD_IMAGE@$WORKLOAD_MANIFEST_DIGEST" &&
  (docker rm -f "$WORKLOAD_CONTAINER_NAME" || true) &&
  docker run -d --name "$WORKLOAD_CONTAINER_NAME" --restart unless-stopped
  -p 8080:8080 "$WORKLOAD_IMAGE@$WORKLOAD_MANIFEST_DIGEST"
```

Running by digest pins the container to exactly the released image.

### Docker Compose with an env file

Combine the [`env_file` strategy](./strategies) with a Compose file that reads
the tag:

```yaml
# compose.yml
services:
  app:
    image: ghcr.io/my-account/app:${IMAGE_TAG}
    container_name: my-app
```

```yaml
# reconD config
container_name: my-app
deploy_command: cd /opt/my-app && docker compose --env-file .env.deploy pull app && docker compose --env-file .env.deploy up -d app
strategy:
  type: env_file
  env_file_path: /opt/my-app/.env.deploy
```

### A script

```sh
#!/bin/sh
# /opt/my-app/deploy.sh
set -eu
docker pull "$WORKLOAD_IMAGE:$WORKLOAD_IMAGE_TAG"
docker stop "$WORKLOAD_CONTAINER_NAME" 2>/dev/null || true
docker rm "$WORKLOAD_CONTAINER_NAME" 2>/dev/null || true
docker run -d --name "$WORKLOAD_CONTAINER_NAME" "$WORKLOAD_IMAGE:$WORKLOAD_IMAGE_TAG"
```

```yaml
deploy_command: /opt/my-app/deploy.sh
```

::: warning Tags must be immutable for rollback
The Compose and script examples deploy by tag. A rollback then redeploys
whatever the previous release's tag points to now, so use immutable tags such
as `prod-sha-<commit>`. If your tags can move, deploy by digest as in the
`docker run` example.
:::

## `container_name` must match

reconD records the release's digest once the command and the health check
succeed; it does not inspect the container at that point. On later cycles,
[drift detection](/guide/how-it-works#drift-detection) inspects
`container_name` to check that the container still runs the recorded digest.
Make sure the command starts a container with exactly that name: if no
container has that name, drift detection never sees it.
