# Changelog

## v0.5.0 - 2026-10-21

### Breaking

- Default config path changed from `/etc/reconD/config.yaml` to `/etc/recond/config.yaml`.
- Default deployments state path changed from `/var/lib/reconD/deployments_state.json` to `/var/lib/recond/deployments_state.json`.

Existing installations that rely on the default paths must migrate before running this version:

- If the old default config path was used, the new version finds no `/etc/recond/config.yaml`, exits with `load config error`, and is restarted by systemd every 5 seconds.
- If only `-config` was passed, the new version starts with an empty state file. Without the previous state, reconD has no rollback target and no recorded terminal failures, so a digest that was previously marked as failed can be deployed again.

Stop and disable any reconD service currently running prior to this version, for example a hand-written `reconD.service`, so it does not start next to the packaged `recond.service` on the next boot. Two agents would reconcile the same workloads with separate state files. Then move the files:

```sh
sudo systemctl disable --now reconD   # the unit name you used before, if any
sudo mkdir -p /etc/recond /var/lib/recond
sudo mv /etc/reconD/config.yaml /etc/recond/config.yaml
sudo mv /var/lib/reconD/deployments_state.json /var/lib/recond/deployments_state.json
```

To keep the old locations instead, override the packaged unit's command with `sudo systemctl edit recond`. The empty `ExecStart=` clears the packaged command before setting the new one:

```ini
[Service]
ExecStart=
ExecStart=/usr/bin/recond -config /etc/reconD/config.yaml -state /var/lib/reconD/deployments_state.json
```

With this override, package installs and upgrades still print setup directions, because `/etc/recond/config.yaml` does not exist.

### Added

- Official `.deb` and `.rpm` packages for `amd64`/`x86_64` and `arm64`/`aarch64`, published on GitHub Releases with `checksums.txt`. Tags with a prerelease suffix, such as `v0.5.0-rc.1`, are published as prereleases.
- Packages install the `recond` binary to `/usr/bin/recond`, a `recond.service` systemd unit, `/etc/recond/config.yaml.example`, and `/var/lib/recond` (mode `0750`).
- `-version` flag that prints the build version, commit, and date.
- The packaged service runs `deploy_command` from `/` with `HOME=/root`. Use absolute paths or start the command with `cd`, for example `cd /opt/my-app && make redeploy-app`.
- Package installs, upgrades, and reinstalls print setup directions until `/etc/recond/config.yaml` exists.
- Packages ship the MIT license as `/usr/share/doc/recond/copyright` (`.deb`) and `/usr/share/licenses/recond/LICENSE` (`.rpm`).
- `apt purge recond` deletes `/etc/recond` and `/var/lib/recond`, including config and deployment state. `apt remove`, `dnf remove`, and `zypper remove` keep them.
- CI lints the package scripts, builds snapshot packages on every pull request, and tests the service lifecycle: `.deb` install, run, reinstall, remove, reinstall, purge, and upgrade of a running service on a systemd host, and `.rpm` install, run, reinstall, erase, and upgrade in Fedora containers with and without a running systemd. Upgrades start from the latest released package, or from a `v0.0.0` snapshot before the first packaged release.
- Releases run the full CI workflow, including the package lifecycle tests, before publishing.

## v0.4.2 - 2026-10-12

### Added

- Added `deployment_retry` configuration for bounded forward deployment and rollback attempts.
- Added top-level `deployment_retry` defaults that apply to every workload.
- Added workload-level `deployment_retry` overrides; workload settings override only the fields they explicitly set.
- Added exponential retry backoff with jitter for deployment strategy, deploy command, health-check, and rollback failures.
- Added persisted terminal failure state for deployment digests that exhaust forward retry attempts, preventing repeated automatic deployment loops.

### Changed

- Forward deployment attempts default to `1`, preserving previous single-attempt behavior unless retry settings are configured.
- Rollback attempts default to `1`, with retry delay defaults of `1s` initial interval, `30s` max interval, and multiplier `2`.
- Rollback now starts only after forward deployment attempts are exhausted.
- Reconciliation now skips automatic deployment when a terminal failure matches the desired manifest digest.
- Reconciliation allows deployment of a newer desired manifest digest when an existing terminal failure belongs to an older digest.
- Successful forward deployment commits now clear terminal failure state for the workload.
- Successful rollback after forward retry exhaustion now persists terminal failure for the failed desired digest, preventing the next reconciliation cycle from retrying the same known-bad digest automatically.
- Deployment state types were clarified: `DeploymentState` is the outer per-workload record, while `DeploymentSnapshot` is the current or previous deployed snapshot.
- Slack success notifications now receive workload name separately from the deployment snapshot.

### Fixed

- Context cancellation during deployment retry backoff and rollback retry backoff now stops retries promptly.
- Unexpected previous-state load errors now abort deployment instead of attempting rollback with an empty previous snapshot.
- Unexpected terminal-failure load errors now abort reconciliation instead of treating terminal failure state as absent.
- Terminal failure records for deployments with no previous state now record zero rollback attempts because no rollback was attempted.
- Rollback commits no longer clear terminal failure state, so a failed desired digest remains blocked even after rollback restores the previous known-good deployment.
- Commit failures during deployment or rollback now surface as deployment errors instead of being reported as successful deployments.
- Backoff delay calculation now clamps safely before duration conversion and caps jittered delays to the configured max interval.

### Upgrade Notes

- Existing central state files remain compatible with the new state layout. Terminal failure state is stored as an optional `terminal_failure` field and is omitted when absent.
- `DeploymentSnapshot` and `TerminalFailureState` no longer carry `workload_name`; the outer `DeploymentState.workload_name` identifies the workload.

## v0.4.1 - 2026-10-08

### Changed

- Deployment state is now managed centrally in one deployments state file for all workloads, keyed by workload name.
- Added the `-state` flag to configure the deployments state file path. It defaults to `/var/lib/reconD/deployments_state.json`.
- Workload-level `state_dir` is no longer required and is no longer used.
- Duplicate workload names are now rejected, since workload name is the deployment state key.
- Missing state errors now distinguish between a missing state file and missing deployment state.

### Upgrade Notes

- This is not treated as a breaking config change: existing YAML files that still include `state_dir` continue to load, but `state_dir` is ignored.
- Existing per-workload `deployed.json` and `previous.json` files are not automatically migrated into the new central state file. Operators should migrate state manually or expect the first run with the new state file to behave like no deployment state has been recorded yet.

## v0.4.0 - 2026-10-08

### Breaking Changes

- Logging configuration now lives in the config file under top-level `log` settings instead of `LOG_FORMAT` and `LOG_LEVEL` environment variables.

  ```yaml
  log:
    format: text
    level: debug
  ```

### Changed

- `logger.Setup` now receives `config.Log`, keeping logger configuration with the rest of the application config.

## v0.3.0 - 2026-10-08

### Breaking Changes

- Workload `environment` has been removed from config, deployment metadata, deployment state, and deploy command environment variables.
- Use optional `release_prefix` to scope GitHub release metadata lookup when a repository publishes multiple deployable artifact streams.

  ```yaml
  workloads:
    - name: my-app
      release_prefix: my-app-prod
  ```

### Changed

- Notifications now identify deployments by workload name instead of environment.
- When `release_prefix` is empty, GitHub release lookup considers all releases newest-first and deploys the first release with valid `deployment-metadata.json`.

## v0.2.0 - 2026-10-08

### Breaking Changes

- Configuration files must now be YAML and use a `.yaml` or `.yml` extension.
- JSON configuration files are no longer supported.
- Config files must define a top-level `workloads` list, even for a single workload.

  ```yaml
  workloads:
    - name: my-app
      environment: prod
  ```

- Single-workload shorthand YAML is no longer supported.

  ```yaml
  # No longer supported:
  name: my-app
  environment: prod
  ```

### Changed

- Updated config loading, tests, and README examples for YAML-only configuration.
