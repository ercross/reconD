package deployment

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ercross/reconD/config"
	"github.com/ercross/reconD/git_provider"
	"github.com/ercross/reconD/notifier"
	"github.com/ercross/reconD/state"
	"github.com/stretchr/testify/require"
)

type fakeStateManager struct {
	loadDeployedErr error
	deployed        state.DeploymentState
	committed       state.DeploymentState
	committedName   string
	rollback        state.DeploymentState
	rollbackName    string
}

func (m *fakeStateManager) LoadDeployed(string) (state.DeploymentState, error) {
	return m.deployed, m.loadDeployedErr
}

func (m *fakeStateManager) LoadPrevious(string) (state.DeploymentState, error) {
	return state.DeploymentState{}, nil
}

func (m *fakeStateManager) CommitDeployed(deploymentName string, s state.DeploymentState) error {
	m.committedName = deploymentName
	m.committed = s
	return nil
}

func (m *fakeStateManager) CommitRollback(deploymentName string, s state.DeploymentState) error {
	m.rollbackName = deploymentName
	m.rollback = s
	return nil
}

func TestDockerDeployerDeployCommitsDeploymentStateForWorkload(t *testing.T) {
	t.Parallel()

	stateMgr := &fakeStateManager{}
	workload := config.Workload{
		Name:          "api",
		ContainerName: "api",
		DeployCommand: "true",
	}
	meta := git_provider.DeploymentMetadata{
		Image:          "ghcr.io/example/api",
		ImageTag:       "v1.0.0",
		ManifestDigest: "sha256:api",
		GitSHA:         "abc123",
	}

	deployer := testDockerDeployer(workload)
	newState, err := deployer.deploy(context.Background(), workload, meta, stateMgr)
	require.NoError(t, err)

	require.Equal(t, "api", stateMgr.committedName)
	require.Equal(t, "api", stateMgr.committed.WorkloadName)
	require.Equal(t, meta.ManifestDigest, stateMgr.committed.ManifestDigest)
	require.Equal(t, newState, stateMgr.committed)
}

func TestDockerDeployerDeployTreatsMissingFileAsNoPreviousState(t *testing.T) {
	t.Parallel()

	stateMgr := &fakeStateManager{loadDeployedErr: state.ErrFileNotFound}
	workload := config.Workload{
		Name:          "api",
		ContainerName: "api",
		DeployCommand: "false",
	}

	err := testDockerDeployer(workload).Deploy(context.Background(), workload, testDeploymentMetadata(), stateMgr)
	require.ErrorContains(t, err, "rollback failed: no previous deployment state")
	require.Empty(t, stateMgr.rollbackName)
}

func TestDockerDeployerDeployTreatsMissingDeploymentAsNoPreviousState(t *testing.T) {
	t.Parallel()

	stateMgr := &fakeStateManager{loadDeployedErr: state.ErrDeploymentStateNotFound}
	workload := config.Workload{
		Name:          "api",
		ContainerName: "api",
		DeployCommand: "false",
	}

	err := testDockerDeployer(workload).Deploy(context.Background(), workload, testDeploymentMetadata(), stateMgr)
	require.ErrorContains(t, err, "rollback failed: no previous deployment state")
	require.Empty(t, stateMgr.rollbackName)
}

func TestDockerDeployerDeployRollsBackToPreviousState(t *testing.T) {
	t.Parallel()

	deployCommand := firstCallFailsThenSucceedsCommand(t)
	previousState := state.DeploymentState{
		WorkloadName:   "api",
		Image:          "ghcr.io/example/api",
		ImageTag:       "v0.9.0",
		ManifestDigest: "sha256:previous",
		GitSHA:         "previous-sha",
		DeployedAt:     time.Now().UTC(),
	}
	stateMgr := &fakeStateManager{deployed: previousState}
	workload := config.Workload{
		Name:          "api",
		ContainerName: "api",
		DeployCommand: deployCommand,
	}
	meta := testDeploymentMetadata()

	err := testDockerDeployer(workload).Deploy(context.Background(), workload, meta, stateMgr)
	require.NoError(t, err)

	require.Equal(t, "api", stateMgr.rollbackName)
	require.Equal(t, "api", stateMgr.rollback.WorkloadName)
	require.Equal(t, previousState.ManifestDigest, stateMgr.rollback.ManifestDigest)
	require.Equal(t, meta.ManifestDigest, stateMgr.rollback.RollbackFrom)
}

func testDockerDeployer(workload config.Workload) *dockerDeployer {
	return NewDockerDeployer(
		workload,
		notifier.Noop{},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).(*dockerDeployer)
}

func testDeploymentMetadata() git_provider.DeploymentMetadata {
	return git_provider.DeploymentMetadata{
		Image:          "ghcr.io/example/api",
		ImageTag:       "v1.0.0",
		ManifestDigest: "sha256:desired",
		GitSHA:         "desired-sha",
	}
}

func firstCallFailsThenSucceedsCommand(t *testing.T) string {
	t.Helper()

	counterPath := filepath.Join(t.TempDir(), "counter")
	scriptPath := filepath.Join(t.TempDir(), "deploy.sh")
	script := fmt.Sprintf(`#!/bin/sh
if [ ! -f %[1]q ]; then
  echo 1 > %[1]q
  exit 1
fi
exit 0
`, counterPath)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o700))
	return scriptPath
}
