---
title: "Deployment metadata: publish releases from GitHub Actions"
description: "The deployment-metadata.json format reconD reads from GitHub Releases, how releases are selected, and how to publish it from GitHub Actions."
---

# Deployment metadata

Deployment metadata is the desired state for a workload. Your CI publishes it
as a GitHub Release asset named exactly `deployment-metadata.json`, after the
image has been pushed to a registry.

## Format

```json
{
  "image": "ghcr.io/my-account/app-repo",
  "image_tag": "prod-sha-10a3e42",
  "manifest_digest": "sha256:19779d908d890f704a2170d7dde679e266a9137613ea299b38939eb889545f8e",
  "git_sha": "10a3e42cc477e9e6da84037474d45c8b1300e9f8",
  "created_at": "2026-05-26T04:47:49Z"
}
```

| Field | Required | Description |
| --- | --- | --- |
| `image` | Yes | Image repository, without a tag. |
| `manifest_digest` | Yes | Immutable OCI manifest digest of the pushed image. This is the key reconD uses for drift detection. |
| `image_tag` | For `env_file` | Tag of the pushed image. Required when the workload uses the [`env_file` strategy](/config/strategies). |
| `git_sha` | No | Source commit that produced the image. Recorded in state and notifications. |
| `created_at` | No | RFC 3339 time CI published the metadata. |

A release whose metadata is missing `image` or `manifest_digest` fails the
cycle with an error, and reconD tries again on the next cycle. The asset must
be smaller than 64 KB.

## How reconD selects a release

On each cycle reconD lists the **30 most recent releases** of the configured
repository, newest first, and uses the first one that:

1. has a tag starting with the workload's `release_prefix` (any tag matches
   when `release_prefix` is empty), and
2. has a `deployment-metadata.json` asset.

Releases without the asset are skipped, so manually created releases do not
interfere.

::: warning Drafts and prereleases are not skipped
reconD does not check whether a release is a draft or a prerelease. GitHub
lists draft releases to tokens with push access, and prereleases to everyone,
so a draft or `-rc` release that carries `deployment-metadata.json` is deployed
like any other. Use a read-only token, and attach the asset only to releases
you want deployed.
:::

### Release prefixes

Set `release_prefix` whenever one repository publishes more than one
deployable stream: several services, or several environments of one service.

```yaml
workloads:
  - name: api
    release_prefix: api-prod        # matches api-prod-sha-10a3e42
  - name: worker
    release_prefix: worker-staging  # matches worker-staging-sha-8d1af01
```

Without a prefix, a release for `worker` could be deployed as `api`.

::: warning
The match is a plain string prefix. `api-prod` also matches `api-prod-eu-...`.
Pick prefixes that are not prefixes of each other.
:::

## Authentication and rate limits

Set `git_provider.token`, or the `GITHUB_TOKEN` environment variable, for
private repositories. Use a token for public repositories too:
unauthenticated GitHub API requests are limited to 60 per hour per IP address.
Each cycle that finds a release makes two requests, one to list releases and
one to download the asset, so a single workload at the default 60s
`check_interval` already exceeds that limit.

A fine-grained token with read-only **Contents** access to the metadata
repository is enough.

## Publishing metadata from GitHub Actions

reconD only requires that the asset exists. This job is one way to publish
it, after building and pushing an image to GHCR with
`docker/build-push-action`:

```yaml
permissions:
  contents: write   # create releases
  packages: write   # push to GHCR

steps:
  - uses: actions/checkout@v6

  # GHCR requires a lowercase repository name.
  - name: Set image name
    run: echo "IMAGE=ghcr.io/${GITHUB_REPOSITORY,,}" >> "$GITHUB_ENV"

  - name: Log in to GHCR
    uses: docker/login-action@v3
    with:
      registry: ghcr.io
      username: ${{ github.actor }}
      password: ${{ github.token }}

  - name: Build and push
    id: build
    uses: docker/build-push-action@v6
    with:
      push: true
      tags: ${{ env.IMAGE }}:prod-sha-${{ github.sha }}

  - name: Publish deployment metadata
    env:
      GH_TOKEN: ${{ github.token }}
      TAG: api-prod-sha-${{ github.sha }}
    run: |
      cat > deployment-metadata.json <<EOF
      {
        "image": "${IMAGE}",
        "image_tag": "prod-sha-${{ github.sha }}",
        "manifest_digest": "${{ steps.build.outputs.digest }}",
        "git_sha": "${{ github.sha }}",
        "created_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
      }
      EOF
      gh release create "$TAG" deployment-metadata.json --target "$GITHUB_SHA" --title "$TAG" --notes ""
```

The job needs `contents: write` to create releases and `packages: write` to
push to GHCR. `--target` tags the commit that was built, rather than the tip
of the default branch.
