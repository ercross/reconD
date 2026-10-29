---
title: "Packaging internals"
description: "How reconD's Debian and RPM package scripts manage the systemd service across install, upgrade, reinstall, removal, and purge."
---

# Packaging internals

How the `.deb` and `.rpm` packages manage `recond.service` across install,
upgrade, and removal. This is for maintainers; operators should read
[Upgrade and uninstall](/install/upgrade).

<!-- The rest of this page is packaging/README.md, from its third line on. -->
<!--@include: ../../packaging/README.md{3,}-->
