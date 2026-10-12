package config

import (
	"math"
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
log:
  format: text
  level: debug
workloads:
  - name: api
    release_prefix: api-prod
    container_name: api
    deploy_command: docker stop api || true && docker rm api || true && docker run -d --name api "$WORKLOAD_IMAGE_REF"
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
    check_interval: 2m
    git_provider:
      owner: toughbred
      repo: gymportal
`), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, "text", cfg.Log.Format)
	require.Equal(t, "debug", cfg.Log.Level)
	require.Len(t, cfg.Workloads, 2)
	require.Equal(t, "api", cfg.Workloads[0].Name)
	require.Equal(t, "api-prod", cfg.Workloads[0].ReleasePrefix)
	require.Equal(t, 45*time.Second, cfg.Workloads[0].CheckInterval.Duration)
	require.Equal(t, HealthCheckTCP, cfg.Workloads[0].HealthCheck.Type)
	require.Equal(t, "worker", cfg.Workloads[1].Name)
	require.Empty(t, cfg.Workloads[1].ReleasePrefix)
	require.Equal(t, 2*time.Minute, cfg.Workloads[1].CheckInterval.Duration)
}

func TestLoadDeploymentRetryConfig(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "workloads.yml")
	require.NoError(t, os.WriteFile(path, []byte(`
workloads:
  - name: api
    container_name: api
    deploy_command: docker restart api
    git_provider:
      owner: toughbred
      repo: gymportal
    deployment_retry:
      max_retry_forward_deployment: 3
      max_retry_rollback: 2
      initial_interval: 100ms
      max_interval: 1s
      multiplier: 1.5
`), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, 3, cfg.Workloads[0].DeploymentRetry.MaxRetryForwardDeployment)
	require.Equal(t, 2, cfg.Workloads[0].DeploymentRetry.MaxRetryRollback)
	require.Equal(t, 100*time.Millisecond, cfg.Workloads[0].DeploymentRetry.InitialInterval.Duration)
	require.Equal(t, time.Second, cfg.Workloads[0].DeploymentRetry.MaxInterval.Duration)
	require.Equal(t, 1.5, cfg.Workloads[0].DeploymentRetry.Multiplier)
}

func TestLoadDefaultsDeploymentRetryConfig(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "workloads.yml")
	require.NoError(t, os.WriteFile(path, []byte(`
workloads:
  - name: api
    container_name: api
    deploy_command: docker restart api
    git_provider:
      owner: toughbred
      repo: gymportal
`), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, 1, cfg.Workloads[0].DeploymentRetry.MaxRetryForwardDeployment)
	require.Equal(t, 1, cfg.Workloads[0].DeploymentRetry.MaxRetryRollback)
	require.Equal(t, time.Second, cfg.Workloads[0].DeploymentRetry.InitialInterval.Duration)
	require.Equal(t, 30*time.Second, cfg.Workloads[0].DeploymentRetry.MaxInterval.Duration)
	require.Equal(t, float64(2), cfg.Workloads[0].DeploymentRetry.Multiplier)
}

func TestLoadTopLevelDeploymentRetryDefaultsApplyToWorkloads(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "workloads.yml")
	require.NoError(t, os.WriteFile(path, []byte(`
deployment_retry:
  max_retry_forward_deployment: 4
  max_retry_rollback: 3
  initial_interval: 250ms
  max_interval: 5s
  multiplier: 1.25
workloads:
  - name: api
    container_name: api
    deploy_command: docker restart api
    git_provider:
      owner: toughbred
      repo: gymportal
  - name: worker
    container_name: worker
    deploy_command: docker restart worker
    git_provider:
      owner: toughbred
      repo: gymportal
    deployment_retry:
      max_retry_forward_deployment: 2
      initial_interval: 100ms
`), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)

	require.Equal(t, 4, cfg.DeploymentRetry.MaxRetryForwardDeployment)
	require.Equal(t, 3, cfg.DeploymentRetry.MaxRetryRollback)
	require.Equal(t, 250*time.Millisecond, cfg.DeploymentRetry.InitialInterval.Duration)
	require.Equal(t, 5*time.Second, cfg.DeploymentRetry.MaxInterval.Duration)
	require.Equal(t, 1.25, cfg.DeploymentRetry.Multiplier)

	require.Equal(t, cfg.DeploymentRetry, cfg.Workloads[0].DeploymentRetry)
	require.Equal(t, 2, cfg.Workloads[1].DeploymentRetry.MaxRetryForwardDeployment)
	require.Equal(t, 3, cfg.Workloads[1].DeploymentRetry.MaxRetryRollback)
	require.Equal(t, 100*time.Millisecond, cfg.Workloads[1].DeploymentRetry.InitialInterval.Duration)
	require.Equal(t, 5*time.Second, cfg.Workloads[1].DeploymentRetry.MaxInterval.Duration)
	require.Equal(t, 1.25, cfg.Workloads[1].DeploymentRetry.Multiplier)
}

func TestDeploymentRetryValidateRejectsNonFiniteMultiplier(t *testing.T) {
	t.Parallel()

	base := DeploymentRetry{
		MaxRetryForwardDeployment: 1,
		MaxRetryRollback:          1,
		InitialInterval:           Duration{Duration: time.Second},
		MaxInterval:               Duration{Duration: time.Second},
		Multiplier:                2,
	}

	for _, multiplier := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		retry := base
		retry.Multiplier = multiplier
		require.ErrorContains(t, retry.validate(), "finite number")
	}
}

func TestLoadRejectsDuplicateWorkloadName(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "workloads.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
workloads:
  - name: api
    container_name: api
    deploy_command: docker stop api || true && docker rm api || true && docker run -d --name api "$WORKLOAD_IMAGE_REF"
    git_provider:
      owner: toughbred
      repo: gymportal
  - name: api
    container_name: api-v2
    deploy_command: docker stop api-v2 || true && docker rm api-v2 || true && docker run -d --name api-v2 "$WORKLOAD_IMAGE_REF"
    git_provider:
      owner: toughbred
      repo: gymportal
`), 0o600))

	_, err := Load(path)
	require.ErrorContains(t, err, `duplicate name "api"`)
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
