---
title: "Deployment strategies: write the image tag to an env file"
description: "Use reconD's env_file strategy to write the released image tag into an env file before Docker Compose or a script deploys it."
---

# Deployment strategies

A strategy prepares the host before `deploy_command` runs, by writing release
metadata where your deploy tooling expects it. Strategies run on every forward
deployment attempt and on every rollback attempt.

| `strategy.type` | Effect |
| --- | --- |
| `none` (default) | Nothing; `deploy_command` runs directly. |
| `env_file` | Writes the image tag into an env file. |

If you do not need a file, you may not need a strategy at all:
`deploy_command` already receives the release in
[`WORKLOAD_*` environment variables](/reference/deploy-command-env).

## `env_file`

```yaml
strategy:
  type: env_file
  env_file_path: /opt/my-app/.env.deploy
  image_tag_key: IMAGE_TAG   # optional, this is the default
```

Before each deploy, reconD sets this line in the file:

```text
IMAGE_TAG=<image_tag from deployment metadata>
```

- If the file does not exist, reconD creates it, along with missing parent
  directories (mode `0750`).
- If the key already exists, its line is replaced in place. Lines written as
  `export IMAGE_TAG=...` are recognized and replaced.
- Every other line, including comments, is kept as is.
- The file is written atomically (written to `<path>.tmp`, then renamed) with
  mode `0600`.

The release's metadata must include `image_tag`; otherwise the strategy fails,
which counts as a deployment failure.

::: warning Rollback relies on immutable tags
This strategy writes only the tag, so a rollback redeploys whatever the
previous release's tag points to now. If you reuse tags such as
`prod-latest`, that is the image that just failed. Use immutable tags, for
example `prod-sha-<commit>`, or deploy by digest with
`$WORKLOAD_IMAGE@$WORKLOAD_MANIFEST_DIGEST`.
:::

This is useful when the deploy command delegates to a script or Compose file
that reads the tag from an env file. See the
[Compose example](./deploy-command#docker-compose-with-an-env-file).

::: tip
Keep the strategy's env file separate from files that hold secrets or other
settings, so that reconD only ever rewrites deployment metadata.
:::
