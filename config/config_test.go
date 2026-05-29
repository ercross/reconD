package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadSingleWorkloadYAML(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "workload.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
name: gymportal-api
environment: prod
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

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Len(t, cfg.Workloads, 1)
	require.Equal(t, "gymportal-api", cfg.Workloads[0].Name)
	require.Equal(t, 30*time.Second, cfg.Workloads[0].CheckInterval.Duration)
	require.Equal(t, 12, cfg.Workloads[0].HealthCheck.Retries)
	require.Equal(t, "IMAGE_TAG", cfg.Workloads[0].Strategy.ImageTagKey)
}

func TestLoadWorkloadListJSON(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "workloads.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
  "workloads": [
    {
      "name": "api",
      "environment": "prod",
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

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Len(t, cfg.Workloads, 1)
	require.Equal(t, 45*time.Second, cfg.Workloads[0].CheckInterval.Duration)
	require.Equal(t, HealthCheckTCP, cfg.Workloads[0].HealthCheck.Type)
}

func TestLoadRejectsTCPHealthCheckWithoutURL(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "workload.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
name: api
environment: prod
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
