---
title: "Build reconD from source"
description: "Build the reconD Go binary from source and install it with its systemd unit on any Linux host."
---

# Build from source

Building from source is supported for contributors and anyone who prefers a
manual install. Unlike with the packages, nothing is installed, created, or
upgraded for you.

You need the Go version listed in
[`go.mod`](https://github.com/ercross/reconD/blob/master/go.mod).

```sh
git clone https://github.com/ercross/reconD.git
cd reconD
go build -o recond .
sudo install -m 0755 recond /usr/bin/recond
sudo install -m 0644 packaging/systemd/recond.service /usr/lib/systemd/system/recond.service
sudo systemctl daemon-reload
sudo install -d -m 0755 /etc/recond
sudo install -m 0600 packaging/config/config.yaml.example /etc/recond/config.yaml
```

Then edit `/etc/recond/config.yaml` with `sudoedit` (see
[Configuration](/config/) for every field), keep its mode at `600`, and
[start the service](./service).

To upgrade, pull, rebuild, reinstall the binary, and restart the service. A
source build reports its version as `dev`.

To build the release packages locally, see
[Development](/contributing/development#building-packages).
