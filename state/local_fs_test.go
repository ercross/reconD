package state

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLocalFileSystemManagerCommitDeployedWritesFirstDeployment(t *testing.T) {
	t.Parallel()

	manager, err := NewManagerWithLocalFileSystem(t.TempDir())
	require.NoError(t, err)

	expected := DeploymentState{
		Environment:    "dev",
		Image:          "ghcr.io/example/app:v1.0.0",
		ManifestDigest: "v1.0.0",
		GitSHA:         "sha-v1.0.0",
		DeployedAt:     time.Now().UTC(),
	}

	require.NoError(t, manager.CommitDeployed(expected))

	actual, err := manager.LoadDeployed()
	require.NoError(t, err)
	require.Equal(t, expected.Environment, actual.Environment)
	require.Equal(t, expected.Image, actual.Image)
	require.Equal(t, expected.ManifestDigest, actual.ManifestDigest)
	require.Equal(t, expected.GitSHA, actual.GitSHA)

	_, err = manager.LoadPrevious()
	require.ErrorIs(t, err, ErrFileNotFound)
}

func TestLocalFileSystemManagerCommitDeployedPromotesPreviousDeployment(t *testing.T) {
	t.Parallel()

	manager, err := NewManagerWithLocalFileSystem(t.TempDir())
	require.NoError(t, err)

	first := DeploymentState{
		Environment:    "dev",
		Image:          "ghcr.io/example/app:v1.0.0",
		ManifestDigest: "v1.0.0",
		GitSHA:         "sha-v1.0.0",
		DeployedAt:     time.Now().UTC(),
	}
	second := DeploymentState{
		Environment:    "dev",
		Image:          "ghcr.io/example/app:v1.1.0",
		ManifestDigest: "v1.1.0",
		GitSHA:         "sha-v1.1.0",
		DeployedAt:     time.Now().UTC(),
	}

	require.NoError(t, manager.CommitDeployed(first))
	require.NoError(t, manager.CommitDeployed(second))

	deployed, err := manager.LoadDeployed()
	require.NoError(t, err)
	require.Equal(t, second.ManifestDigest, deployed.ManifestDigest)

	previous, err := manager.LoadPrevious()
	require.NoError(t, err)
	require.Equal(t, first.ManifestDigest, previous.ManifestDigest)
}
