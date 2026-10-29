---
title: "Deploy command environment variables"
description: "The WORKLOAD_* environment variables reconD passes to your deploy command: image, tag, manifest digest, git SHA, and container name."
---

# Deploy command environment

`deploy_command` runs with reconD's own environment plus these variables,
which describe the release being deployed. During a
[rollback](/operations/rollback), they describe the release being restored.

| Variable | Source | Example |
| --- | --- | --- |
| `WORKLOAD_NAME` | config `name` | `my-app` |
| `WORKLOAD_CONTAINER_NAME` | config `container_name` | `my-app` |
| `WORKLOAD_IMAGE` | metadata `image` | `ghcr.io/my-account/app-repo` |
| `WORKLOAD_IMAGE_TAG` | metadata `image_tag` | `prod-sha-10a3e42` |
| `WORKLOAD_MANIFEST_DIGEST` | metadata `manifest_digest` | `sha256:19779d90...` |
| `WORKLOAD_GIT_SHA` | metadata `git_sha` | `10a3e42cc477e9e6...` |

Variables whose metadata field is missing are set to an empty string.

## Example

```sh
#!/bin/sh
set -eu
ref="$WORKLOAD_IMAGE@$WORKLOAD_MANIFEST_DIGEST"
echo "deploying $WORKLOAD_NAME at $WORKLOAD_GIT_SHA ($ref)"
docker pull "$ref"
docker rm -f "$WORKLOAD_CONTAINER_NAME" 2>/dev/null || true
docker run -d --name "$WORKLOAD_CONTAINER_NAME" --restart unless-stopped "$ref"
```

See [Deploy command](/config/deploy-command) for how the command is run.
