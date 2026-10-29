---
title: "Slack deployment notifications"
description: "Send reconD deployment started, succeeded, failed, and rolled-back events to Slack through an incoming webhook."
---

# Notifications

reconD can post deployment events to Slack through an
[incoming webhook](https://api.slack.com/messaging/webhooks). Set
`notification_url` per workload; leave it empty to disable notifications.

```yaml
workloads:
  - name: my-app
    notification_url: https://hooks.slack.com/services/T000/B000/XXXX
```

## Events

| State | Sent when |
| --- | --- |
| `initialize` | A deployment starts. Sent once per deployment, not once per retry. |
| `success` | A forward deployment succeeds, or a rollback succeeds. A rollback success shows the failed digest under **Rollback From**. |
| `failed` | The forward deployment fails after all attempts, including when there is no previous deployment to roll back to. Sent again with `ROLLBACK FAILED` if the rollback also fails. |

Each message shows the workload name, state, error (if any), manifest digest,
image, tag, the digest rolled back from, and a timestamp. The timestamp is in
the `Africa/Lagos` time zone (WAT, UTC+1).

A typical failed release that rolls back successfully produces three
messages: `initialize`, `failed`, then `success` with **Rollback From** set.

## Delivery

Notifications are best effort. Each request times out after 10 seconds, and a
failure to deliver is logged as an error but never affects the deployment.

The webhook URL is a credential. Keep the config file at mode `600`.
