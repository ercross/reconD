# Changelog

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
