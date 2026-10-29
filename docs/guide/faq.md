---
title: "FAQ: continuous deployment and automatic rollback for Docker"
description: "Answers about auto-deploying Docker containers on a single server with reconD: GitOps without Kubernetes, Docker Compose, rollback, private repos, and more."
---

# Frequently asked questions

## General

### What is reconD?

reconD is a small, open-source continuous deployment agent for a single Linux
host running Docker. Your CI publishes a `deployment-metadata.json` file on a
GitHub Release; reconD notices the new image digest, runs your deploy command,
waits for a health check, and rolls back to the last good release if the new
one fails. See [What is reconD?](./what-is-recond)

### Can I do GitOps without Kubernetes?

Yes. That is the problem reconD solves. Tools such as Argo CD and Flux apply a
reconciliation loop to Kubernetes clusters. reconD applies the same idea, desired
state versus observed state, to plain Docker on one server, without a cluster.

The desired state lives in GitHub Releases rather than in manifests committed
to a git repository, so reconD is GitOps-style rather than strict GitOps. Every
deployment still traces back to a release and a git commit.

### Is reconD free?

Yes. reconD is open source under the MIT license.

### Which platforms does reconD support?

Linux on `amd64`/`x86_64` and `arm64`/`aarch64`, with packages for Debian,
Ubuntu, Fedora, RHEL-compatible distributions, and openSUSE. It needs Docker
and systemd. See [Installation](/install/).

## Deploying

### How do I automatically deploy a Docker container when CI pushes a new image?

1. In CI, after pushing the image, publish a GitHub Release with a
   [`deployment-metadata.json`](./deployment-metadata#publishing-metadata-from-github-actions)
   asset that names the image and its digest.
2. On the server, [install reconD](/install/) and configure a workload with a
   [deploy command](/config/deploy-command) that runs the image.

reconD checks for new releases every 60 seconds by default and deploys the
newest one. If several releases are published between checks, only the newest
is deployed.

### Does reconD work with Docker Compose?

Yes. Use a deploy command such as
`cd /opt/my-app && docker compose --env-file .env.deploy pull app && docker compose --env-file .env.deploy up -d app`,
and the
[`env_file` strategy](/config/strategies) to write the new image tag into an env
file that your Compose file reads. See the
[Compose example](/config/deploy-command#docker-compose-with-an-env-file).

reconD does not parse or manage the Compose file. It only runs your command.

### Can reconD deploy several apps on one server?

Yes. Configure one entry under `workloads` per app. Each workload has its own
repository or [release prefix](./deployment-metadata#release-prefixes), deploy
command, health check, and retry policy, and they are reconciled
independently.

### Does reconD work with private GitHub repositories and registries?

Yes. Set `git_provider.token`, or `GITHUB_TOKEN`, to a token with read access
to the repository that holds the releases. For a private registry, log in once
as root with `sudo docker login`; your deploy command's `docker pull` uses
those credentials.

### Do I need to open any ports or set up webhooks?

No. reconD polls GitHub over outbound HTTPS, so it works on servers behind NAT
or a firewall that blocks inbound traffic.

### Does reconD support GitLab, Bitbucket, or other registries?

reconD reads deployment metadata from GitHub Releases only. The image itself can
be in any registry Docker can pull from, such as GHCR, Docker Hub, ECR, or a
self-hosted registry, because your deploy command does the pull.

## Rollback and reliability

### How do I roll back a Docker container automatically if a deployment fails?

Configure an [HTTP or TCP health check](/config/health-checks). If the deploy
command fails, or the health check does not pass, reconD
[retries](/config/retries) as configured and then redeploys the previous
release. See [Rollback](/operations/rollback).

### Will reconD keep redeploying a broken release?

No. Once a release exhausts its attempts, reconD records it as a
[terminal failure](/operations/rollback#terminal-failures) and stops deploying
that digest automatically. Publishing a newer release unblocks deployment.

### Does reconD restart a container that crashes or stops?

No. Restarting containers is Docker's job: use a restart policy such as
`--restart unless-stopped`. reconD only changes which image version is
deployed.

### What happens if the server reboots during a deployment?

If reconD is interrupted, the state file still holds the last committed state,
because writes go to a temporary file that is renamed into place. The file is
not fsynced, though, so a sudden power loss can leave it empty; see
[Durability](/operations/state#durability).
After the reboot, reconD compares the recorded state with the latest release
again, and deploys if they still differ.

## Comparisons

### How is reconD different from Watchtower?

Watchtower watches the registry and recreates a container whenever its tag
points to a new image. reconD deploys only what your CI explicitly released,
identifies releases by immutable digest, runs your own deploy command, and
health-checks and rolls back failed deployments. See the
[comparison on the home page](/#how-recond-compares).

### Should I use Argo CD or Flux instead?

Use Argo CD or Flux if you run Kubernetes. They reconcile whole clusters from
manifests in git. reconD is for teams that run Docker on a single server and
want the same automated, auditable deployments without operating a cluster.

### Is reconD a replacement for Kubernetes or Docker Swarm?

No. reconD does not schedule containers, manage networking, scale, or
coordinate multiple hosts. It keeps one host running the image versions your
CI released.
