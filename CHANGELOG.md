# Changelog

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
