package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLocalFileSystemManagerCommitDeployedWritesFirstDeployment(t *testing.T) {
	t.Parallel()

	stateFile := filepath.Join(t.TempDir(), "deployments_state.json")
	manager, err := NewManagerWithLocalFileSystem(stateFile)
	require.NoError(t, err)

	expected := DeploymentSnapshot{
		Image:          "ghcr.io/example/app:v1.0.0",
		ManifestDigest: "v1.0.0",
		GitSHA:         "sha-v1.0.0",
		DeployedAt:     time.Now().UTC(),
	}

	require.NoError(t, manager.CommitDeployed("api", expected))

	actual, err := manager.LoadDeployed("api")
	require.NoError(t, err)
	require.Equal(t, expected.Image, actual.Image)
	require.Equal(t, expected.ManifestDigest, actual.ManifestDigest)
	require.Equal(t, expected.GitSHA, actual.GitSHA)

	_, err = manager.LoadPrevious("api")
	require.ErrorIs(t, err, ErrDeploymentStateNotFound)
}

func TestLocalFileSystemManagerCommitDeployedPromotesPreviousDeployment(t *testing.T) {
	t.Parallel()

	stateFile := filepath.Join(t.TempDir(), "deployments_state.json")
	manager, err := NewManagerWithLocalFileSystem(stateFile)
	require.NoError(t, err)

	first := DeploymentSnapshot{
		Image:          "ghcr.io/example/app:v1.0.0",
		ManifestDigest: "v1.0.0",
		GitSHA:         "sha-v1.0.0",
		DeployedAt:     time.Now().UTC(),
	}
	second := DeploymentSnapshot{
		Image:          "ghcr.io/example/app:v1.1.0",
		ManifestDigest: "v1.1.0",
		GitSHA:         "sha-v1.1.0",
		DeployedAt:     time.Now().UTC(),
	}

	require.NoError(t, manager.CommitDeployed("api", first))
	require.NoError(t, manager.CommitDeployed("api", second))

	deployed, err := manager.LoadDeployed("api")
	require.NoError(t, err)
	require.Equal(t, second.ManifestDigest, deployed.ManifestDigest)

	previous, err := manager.LoadPrevious("api")
	require.NoError(t, err)
	require.Equal(t, first.ManifestDigest, previous.ManifestDigest)
}

func TestLocalFileSystemManagerStoresMultipleDeploymentsInOneFile(t *testing.T) {
	t.Parallel()

	stateFile := filepath.Join(t.TempDir(), "deployments_state.json")
	manager, err := NewManagerWithLocalFileSystem(stateFile)
	require.NoError(t, err)

	api := DeploymentSnapshot{
		Image:          "ghcr.io/example/api:v1.0.0",
		ManifestDigest: "api-v1.0.0",
		GitSHA:         "api-sha",
		DeployedAt:     time.Now().UTC(),
	}
	worker := DeploymentSnapshot{
		Image:          "ghcr.io/example/worker:v1.0.0",
		ManifestDigest: "worker-v1.0.0",
		GitSHA:         "worker-sha",
		DeployedAt:     time.Now().UTC(),
	}

	require.NoError(t, manager.CommitDeployed("api", api))
	require.NoError(t, manager.CommitDeployed("worker", worker))

	actualAPI, err := manager.LoadDeployed("api")
	require.NoError(t, err)
	require.Equal(t, "api-v1.0.0", actualAPI.ManifestDigest)

	actualWorker, err := manager.LoadDeployed("worker")
	require.NoError(t, err)
	require.Equal(t, "worker-v1.0.0", actualWorker.ManifestDigest)

	raw, err := os.ReadFile(stateFile)
	require.NoError(t, err)

	var persisted deploymentsStateFile
	require.NoError(t, json.Unmarshal(raw, &persisted))
	require.Len(t, persisted.Deployments, 2)
	require.ElementsMatch(t, []string{"api", "worker"}, []string{
		persisted.Deployments[0].WorkloadName,
		persisted.Deployments[1].WorkloadName,
	})
}

func TestLocalFileSystemManagerCommitRollbackPreservesPreviousDeployment(t *testing.T) {
	t.Parallel()

	stateFile := filepath.Join(t.TempDir(), "deployments_state.json")
	manager, err := NewManagerWithLocalFileSystem(stateFile)
	require.NoError(t, err)

	first := DeploymentSnapshot{
		Image:          "ghcr.io/example/api:v1.0.0",
		ManifestDigest: "api-v1.0.0",
		GitSHA:         "api-sha-v1",
		DeployedAt:     time.Now().UTC(),
	}
	second := DeploymentSnapshot{
		Image:          "ghcr.io/example/api:v1.1.0",
		ManifestDigest: "api-v1.1.0",
		GitSHA:         "api-sha-v2",
		DeployedAt:     time.Now().UTC(),
	}
	rollback := DeploymentSnapshot{
		Image:          first.Image,
		ManifestDigest: first.ManifestDigest,
		GitSHA:         first.GitSHA,
		DeployedAt:     time.Now().UTC(),
		RollbackFrom:   second.ManifestDigest,
	}

	require.NoError(t, manager.CommitDeployed("api", first))
	require.NoError(t, manager.CommitDeployed("api", second))
	require.NoError(t, manager.CommitRollback("api", rollback))

	current, err := manager.LoadDeployed("api")
	require.NoError(t, err)
	require.Equal(t, first.ManifestDigest, current.ManifestDigest)
	require.Equal(t, second.ManifestDigest, current.RollbackFrom)

	previous, err := manager.LoadPrevious("api")
	require.NoError(t, err)
	require.Equal(t, first.ManifestDigest, previous.ManifestDigest)
}

func TestLocalFileSystemManagerCommitRollbackPreservesTerminalFailure(t *testing.T) {
	t.Parallel()

	stateFile := filepath.Join(t.TempDir(), "deployments_state.json")
	manager, err := NewManagerWithLocalFileSystem(stateFile)
	require.NoError(t, err)

	first := DeploymentSnapshot{
		Image:          "ghcr.io/example/api:v1.0.0",
		ManifestDigest: "api-v1.0.0",
		GitSHA:         "api-sha-v1",
		DeployedAt:     time.Now().UTC(),
	}
	failure := TerminalFailureState{
		ManifestDigest:            "api-v1.1.0",
		ForwardDeploymentAttempts: 3,
		RollbackAttempts:          1,
		Reason:                    "deployment failed; rollback succeeded",
	}

	require.NoError(t, manager.CommitDeployed("api", first))
	require.NoError(t, manager.CommitTerminalFailure("api", failure))
	require.NoError(t, manager.CommitRollback("api", DeploymentSnapshot{
		Image:          first.Image,
		ManifestDigest: first.ManifestDigest,
		GitSHA:         first.GitSHA,
		DeployedAt:     time.Now().UTC(),
		RollbackFrom:   failure.ManifestDigest,
	}))

	actual, err := manager.LoadTerminalFailure("api")
	require.NoError(t, err)
	require.Equal(t, failure.ManifestDigest, actual.ManifestDigest)
	require.Equal(t, 1, actual.RollbackAttempts)
}

func TestLocalFileSystemManagerTerminalFailureLifecycle(t *testing.T) {
	t.Parallel()

	stateFile := filepath.Join(t.TempDir(), "deployments_state.json")
	manager, err := NewManagerWithLocalFileSystem(stateFile)
	require.NoError(t, err)

	failure := TerminalFailureState{
		ManifestDigest:            "sha256:failed",
		GitSHA:                    "failed-sha",
		ForwardDeploymentAttempts: 3,
		RollbackAttempts:          2,
		Reason:                    "rollback failed",
	}

	require.NoError(t, manager.CommitTerminalFailure("api", failure))

	actual, err := manager.LoadTerminalFailure("api")
	require.NoError(t, err)
	require.Equal(t, failure.ManifestDigest, actual.ManifestDigest)
	require.Equal(t, 3, actual.ForwardDeploymentAttempts)
	require.False(t, actual.FailedAt.IsZero())

	require.NoError(t, manager.ClearTerminalFailure("api"))
	_, err = manager.LoadTerminalFailure("api")
	require.ErrorIs(t, err, ErrDeploymentStateNotFound)
}

func TestLocalFileSystemManagerOmitsAbsentTerminalFailure(t *testing.T) {
	t.Parallel()

	stateFile := filepath.Join(t.TempDir(), "deployments_state.json")
	manager, err := NewManagerWithLocalFileSystem(stateFile)
	require.NoError(t, err)

	require.NoError(t, manager.CommitDeployed("api", DeploymentSnapshot{
		Image:          "ghcr.io/example/api:v1.0.0",
		ManifestDigest: "sha256:ok",
		GitSHA:         "ok-sha",
		DeployedAt:     time.Now().UTC(),
	}))

	raw, err := os.ReadFile(stateFile)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "terminal_failure")
}

func TestLocalFileSystemManagerCommitDeployedClearsTerminalFailure(t *testing.T) {
	t.Parallel()

	stateFile := filepath.Join(t.TempDir(), "deployments_state.json")
	manager, err := NewManagerWithLocalFileSystem(stateFile)
	require.NoError(t, err)

	require.NoError(t, manager.CommitTerminalFailure("api", TerminalFailureState{
		ManifestDigest: "sha256:failed",
		Reason:         "failed",
	}))
	require.NoError(t, manager.CommitDeployed("api", DeploymentSnapshot{
		Image:          "ghcr.io/example/api:v1.0.0",
		ManifestDigest: "sha256:ok",
		GitSHA:         "ok-sha",
		DeployedAt:     time.Now().UTC(),
	}))

	_, err = manager.LoadTerminalFailure("api")
	require.ErrorIs(t, err, ErrDeploymentStateNotFound)
}

func TestLocalFileSystemManagerLoadDeployedMissingFile(t *testing.T) {
	t.Parallel()

	stateFile := filepath.Join(t.TempDir(), "deployments_state.json")
	manager, err := NewManagerWithLocalFileSystem(stateFile)
	require.NoError(t, err)

	_, err = manager.LoadDeployed("api")
	require.ErrorIs(t, err, ErrFileNotFound)
	require.True(t, IsNotFound(err))
}

func TestLocalFileSystemManagerLoadDeployedMissingDeployment(t *testing.T) {
	t.Parallel()

	stateFile := filepath.Join(t.TempDir(), "deployments_state.json")
	manager, err := NewManagerWithLocalFileSystem(stateFile)
	require.NoError(t, err)

	require.NoError(t, manager.CommitDeployed("api", DeploymentSnapshot{
		Image:          "ghcr.io/example/api:v1.0.0",
		ManifestDigest: "api-v1.0.0",
		GitSHA:         "api-sha",
		DeployedAt:     time.Now().UTC(),
	}))

	_, err = manager.LoadDeployed("worker")
	require.ErrorIs(t, err, ErrDeploymentStateNotFound)
	require.NotErrorIs(t, err, ErrFileNotFound)
	require.True(t, IsNotFound(err))
}
