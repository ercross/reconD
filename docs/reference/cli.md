---
title: "CLI reference"
description: "reconD command-line flags, environment variables, signal handling, and exit codes."
---

# CLI

```text
recond [-config <path>] [-state <path>] [-version]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-config` | `/etc/recond/config.yaml` | Path to the [config file](/config/). Must end in `.yaml` or `.yml`. |
| `-state` | `/var/lib/recond/deployments_state.json` | Path to the [deployments state file](/operations/state). Its directory is created if missing. |
| `-version` | | Print the version, commit, and build date, then exit. |

```sh
$ recond -version
recond 0.5.0 (commit 23856ca..., built 2026-10-21T10:12:00Z)
```

A source build prints `recond dev (commit none, built unknown)`.

## Environment

| Variable | Description |
| --- | --- |
| `GITHUB_TOKEN` | GitHub token used for any workload without `git_provider.token`. |

reconD passes its own environment through to `deploy_command` and to the
`docker` commands it runs, so variables such as `DOCKER_HOST` and
`DOCKER_CONFIG` apply to both.

## Signals and exit status

`SIGTERM` and `SIGINT` stop reconD gracefully. It cancels running deploy
commands, health checks, and retry backoffs, waits for every workload's
reconciler to return, and exits `0`.

reconD exits `1`, printing `agent error: ...` to stderr, when the config cannot
be loaded or is invalid, or when the state file's directory cannot be created.
An unknown flag prints usage and exits `2`; `-h` prints usage and exits `0`.
Errors during reconciliation are logged and never stop the process.
