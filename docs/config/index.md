---
title: "Configuration overview"
description: "How reconD's YAML config is structured, with a complete multi-workload example, duration formats, and validation errors."
---

# Configuration overview

reconD reads one YAML config file at startup. The packaged service uses
`/etc/recond/config.yaml`; pass `-config` to use another path (see
[CLI](/reference/cli)).

- The file must have a `.yaml` or `.yml` extension.
- It must define a top-level `workloads` list, even for a single workload.
- Workload names must be unique, because the name keys the workload's
  deployment state.
- Changes take effect only after a restart: `sudo systemctl restart recond`.

## Structure

```yaml
log:                 # optional: process logging
  format: json
  level: info

deployment_retry:    # optional: retry defaults for every workload
  max_retry_forward_deployment: 1
  max_retry_rollback: 1

workloads:           # required: one entry per independently deployed service
  - name: my-app
    container_name: my-app
    deploy_command: /opt/my-app/deploy.sh
    git_provider:
      owner: my-github-owner
      repo: my-app
    labels:
      app: my-app   # must match a LABEL on your image; scopes image pruning
```

## Complete example

```yaml
log:
  format: text
  level: info

deployment_retry:
  max_retry_forward_deployment: 3
  max_retry_rollback: 2
  initial_interval: 1s
  max_interval: 30s
  multiplier: 2

workloads:
  - name: my-app
    release_prefix: my-app-prod
    container_name: my-app
    deploy_command: cd /opt/my-app && make redeploy-app
    check_interval: 60s
    notification_url: https://hooks.slack.com/services/T000/B000/XXXX
    git_provider:
      owner: my-github-owner
      repo: my-deployment-metadata-repo
      token: ""            # falls back to the GITHUB_TOKEN environment variable
    strategy:
      type: env_file
      env_file_path: /opt/my-app/.env.deploy
      image_tag_key: IMAGE_TAG
    health_check:
      type: http
      url: http://localhost:8080/health
      retries: 12
      interval: 10s
      timeout: 5s
    labels:
      app: my-app

  - name: worker
    release_prefix: worker-prod
    container_name: worker
    deploy_command: /opt/worker/deploy.sh
    git_provider:
      owner: my-github-owner
      repo: my-deployment-metadata-repo
    health_check:
      type: tcp
      url: tcp://localhost:9000
    labels:
      app: worker
    deployment_retry:
      max_retry_forward_deployment: 1   # overrides only this field
```

## Durations

Duration fields accept either a Go duration string (`500ms`, `10s`, `1m30s`)
or a plain integer number of seconds (`60`).

## Validation

reconD validates the whole file before starting any workload and exits with
`load config error` if anything is wrong, naming the workload index and field.
For example:

```text
agent error: load config error "/etc/recond/config.yaml": workload 0: deploy_command is required
```

## Where to go next

- [Field reference](./reference): every field, its default, and its rules.
- [Deploy command](./deploy-command): how your command is run.
- [Deployment strategies](./strategies), [Health checks](./health-checks),
  [Retries](./retries), [Notifications](./notifications).
