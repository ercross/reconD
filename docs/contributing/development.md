---
title: "Development and contributing"
description: "Build, test, and run reconD locally, understand the Go package layout, build .deb and .rpm packages, release, and edit these docs."
---

# Development

## Prerequisites

- Go, at the version in [`go.mod`](https://github.com/ercross/reconD/blob/master/go.mod).
- Docker, with the `docker` CLI on `PATH`.
- A GitHub token, if the release metadata you test against is private.

## Build and test

```sh
go build ./...
go vet ./...
go test -race ./...
gofmt -l .        # must print nothing
```

CI runs these same checks on every pull request, plus `go mod tidy` with a
check that `go.mod` and `go.sum` are unchanged.

## Run the agent locally

```sh
GITHUB_TOKEN=your_token_here go run . \
  -config ./my-config.yaml \
  -state ./deployments_state.json
```

Set an explicit `-state` path; the default under `/var/lib/recond` usually
needs root. For readable output, add this to your config:

```yaml
log:
  format: text
  level: debug
```

When run by hand, `deploy_command` runs from your current directory, unlike
under the service, where it runs from `/`.

## Project layout

| Package | Responsibility |
| --- | --- |
| `main.go` | Flags, startup, one reconciler goroutine per workload, signal handling. |
| `config` | YAML loading, defaults, and validation. |
| `reconciler` | The per-workload loop: fetch desired state, detect drift, deploy. |
| `deployment` | Strategy, `deploy_command`, health wait, retries, rollback, and image pruning via the Docker CLI. |
| `git_provider` | Fetching `deployment-metadata.json` from GitHub Releases. |
| `state` | The JSON state file, with atomic writes. |
| `health` | HTTP and TCP health checks. |
| `notifier` | Slack notifications. |
| `logger` | `slog` setup and phase constants. |
| `packaging` | systemd unit, package scripts, and example config. |

## Building packages

Install [GoReleaser](https://goreleaser.com) v2 and run:

```sh
make release-test
```

This runs `goreleaser release --snapshot --clean` and writes `.deb`, `.rpm`,
and `.tar.gz` artifacts to `dist/`. See
[Packaging internals](./packaging) for how the packages manage the service.

## Releasing

Push a tag starting with `v`, for example `v0.6.0`. The release workflow runs
the full CI workflow, including the package lifecycle tests, then publishes the
packages and `checksums.txt` to GitHub Releases. Tags with a prerelease
suffix, such as `v0.6.0-rc.1`, are published as prereleases.

Update [`CHANGELOG.md`](https://github.com/ercross/reconD/blob/master/CHANGELOG.md)
before tagging.

## Documentation

This site lives in `docs/` and is built with [VitePress](https://vitepress.dev).

```sh
cd docs
npm ci
npm run dev       # live preview at http://localhost:5173/reconD/
npm run build     # production build into docs/.vitepress/dist
```

Changes to `docs/`, `CHANGELOG.md`, or `packaging/README.md` on `master` are
published to GitHub Pages automatically. The repository's **Settings → Pages →
Source** must be set to **GitHub Actions**, or the deploy job fails.
