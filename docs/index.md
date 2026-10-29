---
layout: home
title: "reconD – GitOps-style continuous deployment for single-host Docker"
titleTemplate: false
description: "reconD is a lightweight, open-source deployment agent for one Docker host. It deploys the image your CI released, health-checks it, and rolls back on failure."

hero:
  name: reconD
  text: Ship a release, and your server updates itself
  tagline: reconD watches for new releases from your CI, deploys them to your Docker host, and rolls back if something breaks.
  image:
    src: /logo.svg
    alt: reconD
  actions:
    - theme: brand
      text: Get started
      link: /guide/getting-started
    - theme: alt
      text: What is reconD?
      link: /guide/what-is-recond
    - theme: alt
      text: View on GitHub
      link: https://github.com/ercross/reconD

features:
  - title: No more SSH-ing in to deploy
    details: When your CI publishes a new version, reconD deploys it to your server, checks it's healthy, and puts the old version back if it isn't. Neither you nor your CI pipeline needs SSH access to the server.
  - title: Your deploy command, unchanged
    details: reconD runs one shell command per workload — docker run, a script, a Make target, or Compose. It never parses or owns your topology.
  - title: Health checks and rollback
    details: Waits on an HTTP or TCP check after every deploy. If the new version fails, it redeploys the last known-good version automatically.
  - title: Bounded retries
    details: Exponential backoff with jitter, then a persisted terminal failure so a bad release is not retried in a loop.
  - title: Native Linux packages
    details: Ships as .deb and .rpm for amd64 and arm64, with a systemd unit, and config and state in standard locations.
  - title: Intentionally narrow
    details: No clusters, scheduling, networking, or secrets. One job, done predictably.
---

## How reconD compares

reconD sits between registry watchers that update containers automatically and
Kubernetes GitOps controllers that need a cluster.

| | reconD | Watchtower | Argo CD / Flux |
| --- | --- | --- | --- |
| Runs on | One Linux host with Docker | One host with Docker | Kubernetes clusters |
| What triggers a deploy | A release your CI publishes, with `deployment-metadata.json` | A new image pushed to the registry under the running tag | A commit to manifests in a git repository |
| Tracks versions by | Immutable manifest digest | The running image tag | Git commit of the manifests |
| How it deploys | Your own command: `docker run`, Compose, a script, or Make | Recreates the container with its existing options | Applies Kubernetes manifests, Helm charts, or Kustomize |
| Health checks before marking success | HTTP or TCP | No | Kubernetes resource health |
| Automatic rollback | Yes, to the last good release | No | Not by default: revert in git, or use Argo CD's history rollback; automatic with Argo Rollouts, Flagger, or Flux Helm remediation |
| Infrastructure needed | A systemd service | A container with the Docker socket | A cluster and an in-cluster controller |

### reconD vs Watchtower

Watchtower keeps containers on the newest image for their tag: when the
registry has a newer `my-app:latest`, it pulls it and recreates the container.
That is convenient, but every push to the registry goes live, and a broken
image stays running.

reconD deploys only what your CI explicitly released. It pins releases by
digest, deploys them with your own command, waits for a health check, and
rolls back to the previous release if the new one fails. Choose Watchtower to
keep third-party images such as databases or dashboards up to date with no
setup. Choose reconD for your own applications, where each deployment should
be deliberate, traceable to a commit, and safe to fail.

The upstream Watchtower project (`containrrr/watchtower`) was archived in
December 2025; community forks continue it.

### reconD vs Argo CD and Flux

Argo CD and Flux reconcile Kubernetes clusters from manifests in git, and they
are the right choice if you already run Kubernetes. reconD brings the same
reconciliation loop, desired state versus observed state, to a single server
running plain Docker. You get automated, auditable deployments without
operating a cluster, an ingress controller, or in-cluster controllers.

[Read the FAQ](/guide/faq) · [Get started](/guide/getting-started)
