---
title: "What is reconD? Continuous deployment for single-host Docker"
description: "reconD is a GitOps-style deployment agent for one Docker host: it deploys the image your CI released, checks its health, and rolls back on failure."
---

# What is reconD?

reconD is a small deployment agent for single-host container workloads.

It watches deployment metadata that your CI publishes. It compares the image
digest in that metadata with the host's observed state, and runs one
configured deploy command when the two differ.

The project is intentionally narrow. reconD is a reconciliation loop for
workload image state. It is not a container orchestrator.

## When to use it

reconD fits when you already have:

- CI that builds and publishes container images.
- CI that publishes a [`deployment-metadata.json`](./deployment-metadata) asset
  on a GitHub Release alongside each image release.
- A single host that should run one or more named workloads, each at a desired
  image version.
- A local command that knows how to deploy each workload on that host.

The deploy command can be `docker run`, a shell script, a Make target, or a
command that invokes Docker Compose. reconD does not understand or manage the
topology behind that command.

## Where the desired state comes from

The desired state is the deployment metadata emitted alongside an image
release. The container registry is not the source of truth on its own: a new
image tag in the registry does nothing until CI publishes metadata that points
at it.

This gives you an explicit, auditable release step. Whatever publishes the
metadata decides what runs on the host.

## Out of scope

reconD does not manage:

- Docker Compose projects
- Multi-container application topology
- Service dependencies
- Networking
- Volumes
- Secrets
- Ingress or traffic routing
- Load balancing
- Scaling
- Scheduling
- Clusters
- Infrastructure provisioning

If a feature requires understanding application topology, dependency ordering,
networking, routing, or workload composition, it belongs outside the agent.

reconD also does not restart a stopped container. If the container is not
running, reconD reports no drift and leaves it alone. Keeping containers
running belongs to Docker's restart policies or your deploy command.

## Design rule

> reconD ensures a single named workload is running the image specified by its
> deployment metadata.
>
> Everything else belongs to the deploy command or to external tooling.

## Next steps

- [Getting started](./getting-started): install reconD and deploy your first
  workload.
- [How it works](./how-it-works): the reconciliation loop step by step.
