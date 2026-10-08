# Changelog

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
