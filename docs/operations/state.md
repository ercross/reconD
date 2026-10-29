---
title: "Deployment state file"
description: "The format of reconD's deployments_state.json, how deployments and rollbacks change it, and how to safely inspect or edit it."
---

# Deployment state

reconD records what it has deployed in one JSON file shared by all workloads.
The default path is `/var/lib/recond/deployments_state.json`; change it with the
[`-state` flag](/reference/cli).

The file is the agent's memory. It determines drift, the rollback target, and
which releases are blocked as terminal failures. Back it up along with your
config.

## Format

```json
{
  "deployments": [
    {
      "workload_name": "my-app",
      "current": {
        "image": "ghcr.io/my-account/app-repo",
        "image_tag": "prod-sha-10a3e42",
        "manifest_digest": "sha256:19779d90...",
        "git_sha": "10a3e42cc477e9e6da84037474d45c8b1300e9f8",
        "deployed_at": "2026-05-26T05:01:12Z"
      },
      "previous": {
        "image": "ghcr.io/my-account/app-repo",
        "image_tag": "prod-sha-8d1af01",
        "manifest_digest": "sha256:8a1c4e22...",
        "git_sha": "8d1af01b...",
        "deployed_at": "2026-05-20T11:42:03Z"
      }
    }
  ]
}
```

| Field | Description |
| --- | --- |
| `workload_name` | The workload's `name` from the config. |
| `current` | What reconD last deployed successfully, by forward deploy or rollback. |
| `previous` | The deployment before `current`, kept for reference. A rollback restores `current`, the release that was running when the failed deployment started. |
| `current.rollback_from` | Set when `current` was restored by a rollback: the digest that failed. |
| `terminal_failure` | Present when a release exhausted its attempts. See [Rollback and terminal failures](./rollback#terminal-failures). |

## How it changes

| Event | `current` | `previous` | `terminal_failure` |
| --- | --- | --- | --- |
| Successful forward deploy | new release | old `current` | cleared |
| Successful rollback | restored release, with `rollback_from` | unchanged | kept, and set for the failed digest |
| Failed deploy, no rollback possible | unchanged | unchanged | set |
| Failed rollback | unchanged | unchanged | set |

## Durability

Every write goes to `<path>.tmp` and is then renamed over the file, so if
reconD crashes or is killed mid-write, the file holds either the old or the new
state, never a partial one.

The file is not fsynced, so a power loss or kernel crash can still leave it
lost or empty. reconD treats an empty file as having no state: every workload
counts as never deployed, with no rollback target and no terminal failures.
Back the file up.

The file is written with mode `0640`; its directory is created with mode
`0750` if missing.

## Editing the file by hand

You can inspect the file at any time. To edit it, stop the service first, so
reconD does not overwrite your change:

```sh
sudo systemctl stop recond
sudoedit /var/lib/recond/deployments_state.json
sudo systemctl start recond
```

If you delete the file, or a workload's entry, reconD treats that workload as
never deployed. On the next cycle it deploys the desired release, with nothing
to roll back to if that fails.

::: warning Renaming a workload
State is keyed by `name`. Renaming a workload in the config starts it with
empty state. Rename its `workload_name` in the state file at the same time to
keep its history.
:::
