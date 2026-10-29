---
title: "Security model"
description: "Why reconD runs as root, who controls what gets deployed, and how to protect the config, deploy scripts, GitHub token, and Slack webhook."
---

# Security model

## reconD runs as root

The packaged service runs as `root`. reconD drives the host Docker daemon and
runs your `deploy_command`. Access to the Docker daemon is equivalent to root
access on the host, so a dedicated unprivileged user would not meaningfully
reduce what the agent can do.

What follows from that:

- **The config file is code.** Anyone who can edit
  `/etc/recond/config.yaml` can run any command as root through
  `deploy_command`. Keep it owned by root with mode `600`.
- **Scripts called by `deploy_command` are code too.** Keep them, and the
  directories that contain them, writable by root only.
- **Only put commands you trust in `deploy_command`.**

## Who decides what runs

Whoever can publish a GitHub Release with a `deployment-metadata.json` asset in
the metadata repository decides which image the host runs. Treat release
permissions on that repository as deploy permissions:

- Restrict who can create releases, and prefer creating them only from CI.
- Use a `release_prefix` per workload, so a release meant for one workload
  cannot be deployed as another.
- Pin by digest in your deploy command (`$WORKLOAD_IMAGE@$WORKLOAD_MANIFEST_DIGEST`)
  so the container runs exactly the image the metadata names, even if a tag is
  later moved.

The metadata values are passed to `deploy_command` as environment variables,
not interpolated into the command string. Quote them in your scripts
(`"$WORKLOAD_IMAGE"`) as you would any input.

## Credentials

| Credential | Where it lives | Advice |
| --- | --- | --- |
| GitHub token | `git_provider.token` or `GITHUB_TOKEN` | Fine-grained, read-only Contents access to the metadata repository only. |
| Slack webhook URL | `notification_url` | Anyone with the URL can post to the channel. |
| Registry login | Docker's config under `/root/.docker` | Managed outside reconD, for example with `sudo docker login`. |

To keep the GitHub token out of the config file, set it in a systemd
`EnvironmentFile`; see
[Running the service](/install/service#custom-paths-or-flags). A token set this
way is in reconD's environment, so every `deploy_command` and the processes it
starts can read it too. A token in `git_provider.token` is not passed to the
deploy command.

## Running in a container

Mounting `/var/run/docker.sock` into a reconD container grants the same root
equivalence, without the clarity of a host service. See
[Running reconD in a container](/install/service#running-recond-in-a-container).
