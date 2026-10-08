package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadRejectsYAMLWithoutWorkloadsKey(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "workload.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
name: gymportal-api
container_name: gymportal-api
deploy_command: docker stop gymportal-api || true && docker rm gymportal-api || true && docker run -d --name gymportal-api "$WORKLOAD_IMAGE_REF"
state_dir: /tmp/recond/gymportal-api
check_interval: 30s
git_provider:
  owner: toughbred
  repo: gymportal
strategy:
  type: env_file
  env_file_path: /tmp/recond/gymportal-api/.env
health_check:
  type: http
  url: http://localhost:8080/health
`), 0o600))

	_, err := Load(path)
	require.ErrorContains(t, err, "at least one workload must be configured")
}

func TestLoadWorkloadListYAML(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "workloads.yml")
	require.NoError(t, os.WriteFile(path, []byte(`
workloads:
  - name: api
    release_prefix: api-prod
    container_name: api
    deploy_command: docker stop api || true && docker rm api || true && docker run -d --name api "$WORKLOAD_IMAGE_REF"
    state_dir: /tmp/recond/api
    check_interval: 45
    git_provider:
      owner: toughbred
      repo: gymportal
    health_check:
      type: tcp
      url: tcp://localhost:8080
  - name: worker
    container_name: worker
    deploy_command: docker stop worker || true && docker rm worker || true && docker run -d --name worker "$WORKLOAD_IMAGE_REF"
    state_dir: /tmp/recond/worker
    check_interval: 2m
    git_provider:
      owner: toughbred
      repo: gymportal
`), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Len(t, cfg.Workloads, 2)
	require.Equal(t, "api", cfg.Workloads[0].Name)
	require.Equal(t, "api-prod", cfg.Workloads[0].ReleasePrefix)
	require.Equal(t, 45*time.Second, cfg.Workloads[0].CheckInterval.Duration)
	require.Equal(t, HealthCheckTCP, cfg.Workloads[0].HealthCheck.Type)
	require.Equal(t, "worker", cfg.Workloads[1].Name)
	require.Empty(t, cfg.Workloads[1].ReleasePrefix)
	require.Equal(t, 2*time.Minute, cfg.Workloads[1].CheckInterval.Duration)
}

func TestLoadRejectsJSONConfigExtension(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "workloads.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
  "workloads": [
    {
      "name": "api",
      "release_prefix": "api-prod",
      "container_name": "api",
      "deploy_command": "docker stop api || true && docker rm api || true && docker run -d --name api \"$WORKLOAD_IMAGE_REF\"",
      "state_dir": "/tmp/recond/api",
      "check_interval": "45s",
      "git_provider": {
        "owner": "toughbred",
        "repo": "gymportal"
      },
      "health_check": {
        "type": "tcp",
        "url": "tcp://localhost:8080"
      }
    }
  ]
}`), 0o600))

	_, err := Load(path)
	require.ErrorContains(t, err, "unsupported config file extension")
}

func TestLoadRejectsEmptyWorkloadsList(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "workloads.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
workloads: []
`), 0o600))

	_, err := Load(path)
	require.ErrorContains(t, err, "at least one workload must be configured")
}

func TestLoadRejectsWorkloadsMapping(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "workloads.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
workloads:
  name: api
  release_prefix: api-prod
`), 0o600))

	_, err := Load(path)
	require.ErrorContains(t, err, "cannot unmarshal")
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "workloads.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
workloads:
  - name: api
    container_name: api
    deploy_command: docker stop api || true && docker rm api || true && docker run -d --name api "$WORKLOAD_IMAGE_REF"
    state_dir: /tmp/recond/api
    check_interval: sometimes
    git_provider:
      owner: toughbred
      repo: gymportal
`), 0o600))

	_, err := Load(path)
	require.ErrorContains(t, err, "parse duration")
}

func TestLoadRejectsUnsupportedConfigExtension(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "workloads.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
workloads:
  - name: api
`), 0o600))

	_, err := Load(path)
	require.ErrorContains(t, err, "unsupported config file extension")
}

func TestLoadRejectsTCPHealthCheckWithoutURL(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "workload.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
workloads:
  - name: api
    container_name: api
    deploy_command: docker stop api || true && docker rm api || true && docker run -d --name api "$WORKLOAD_IMAGE_REF"
    state_dir: /tmp/recond/api
    git_provider:
      owner: toughbred
      repo: gymportal
    health_check:
      type: tcp
`), 0o600))

	_, err := Load(path)
	require.ErrorContains(t, err, "health_check.url")
}
