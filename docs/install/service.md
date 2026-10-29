---
title: "Run reconD as a systemd service"
description: "Start, stop, and monitor the reconD systemd service, override its flags, pass a GitHub token, and understand its runtime environment."
---

# Running the service

The packages install `recond.service`, a systemd unit that runs:

```sh
/usr/bin/recond -config /etc/recond/config.yaml -state /var/lib/recond/deployments_state.json
```

## Manage the service

```sh
sudo systemctl enable --now recond   # start now and on boot
sudo systemctl status recond
sudo journalctl -u recond -f         # follow logs
sudo systemctl stop recond
sudo systemctl restart recond        # after editing the config
sudo systemctl disable recond        # do not start on boot
```

reconD reads its config only at startup. Restart the service after every
config change.

## Missing or invalid config

Create `/etc/recond/config.yaml` before starting the service. If the file is
missing or invalid, reconD exits with an error and systemd restarts it every 5
seconds, indefinitely, logging the same `load config error` each time.

To stop the loop, run `sudo systemctl stop recond`, fix the config, then start
the service again. `journalctl -u recond` shows the exact error.

## Service environment

The unit runs as `root`; see the [security model](/operations/security) for
why.

Under the service, `deploy_command` runs from `/`, not from the directory you
ran reconD from when testing by hand, and `HOME` is `/root`. Use absolute paths
in `deploy_command` and `strategy.env_file_path`, or start the command with
`cd`:

```yaml
deploy_command: cd /opt/my-app && make redeploy-app
```

A relative `./deploy.sh`, `make` target, or Compose file would otherwise be
looked up in `/`.

## Custom paths or flags

To change the command line, override the unit with `sudo systemctl edit recond`.
The empty `ExecStart=` clears the packaged command before setting a new one:

```ini
[Service]
ExecStart=
ExecStart=/usr/bin/recond -config /srv/recond/config.yaml -state /srv/recond/state.json
```

To pass a GitHub token through the environment instead of the config file, add
an environment file in the same override:

```ini
[Service]
EnvironmentFile=/etc/recond/env
```

with `/etc/recond/env` containing `GITHUB_TOKEN=...` and mode `600`. Every
`deploy_command` inherits reconD's environment, so it can read this token too.

## Running reconD in a container

For production, run reconD as a host-level service from the packages. This
matches the agent's job: it observes host container state, runs the local
`docker` command, writes local state, and executes a `deploy_command` that may
depend on host paths, scripts, Make targets, Compose files, or env files.

Running reconD as a Docker container is possible but is an advanced option. A
containerized agent typically needs the host Docker socket, persistent config
and state mounts, and sometimes host networking or host project directories.
Mounting the Docker socket gives the agent broad control over the host, so the
security boundary is not that of an ordinary isolated container.
