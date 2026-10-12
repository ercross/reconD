package deployment

import (
	"context"
	"errors"
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
	deployed        state.DeploymentSnapshot
	committed       state.DeploymentSnapshot
	committedName   string
	commitErr       error
	rollback        state.DeploymentSnapshot
	rollbackName    string
	rollbackErr     error
	rollbackCommits int
	terminalFailure state.TerminalFailureState
	terminalCommits int
}

func (m *fakeStateManager) LoadDeployed(string) (state.DeploymentSnapshot, error) {
	return m.deployed, m.loadDeployedErr
}

func (m *fakeStateManager) LoadPrevious(string) (state.DeploymentSnapshot, error) {
	return state.DeploymentSnapshot{}, nil
}

func (m *fakeStateManager) LoadTerminalFailure(string) (state.TerminalFailureState, error) {
	return state.TerminalFailureState{}, state.ErrDeploymentStateNotFound
}

func (m *fakeStateManager) CommitDeployed(deploymentName string, s state.DeploymentSnapshot) error {
	m.committedName = deploymentName
	m.committed = s
	return m.commitErr
}

func (m *fakeStateManager) CommitRollback(deploymentName string, s state.DeploymentSnapshot) error {
	m.rollbackName = deploymentName
	m.rollback = s
	m.rollbackCommits++
	return m.rollbackErr
}

func (m *fakeStateManager) CommitTerminalFailure(_ string, s state.TerminalFailureState) error {
	m.terminalFailure = s
	m.terminalCommits++
	return nil
}

func (m *fakeStateManager) ClearTerminalFailure(string) error {
	m.terminalFailure = state.TerminalFailureState{}
	return nil
}

type fakeNotifier struct {
	started  int
	failed   int
	success  int
	lastFail error
}

func (n *fakeNotifier) NotifyOnNewDeploymentStarted(string, git_provider.DeploymentMetadata) {
	n.started++
}

func (n *fakeNotifier) NotifyOnDeploymentFailed(_ string, _ git_provider.DeploymentMetadata, err error) {
	n.failed++
	n.lastFail = err
}

func (n *fakeNotifier) NotifyOnDeploymentSuccess(string, state.DeploymentSnapshot) {
	n.success++
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
	require.ErrorContains(t, err, "no previous deployment state")
	require.Empty(t, stateMgr.rollbackName)
	require.Equal(t, testDeploymentMetadata().ManifestDigest, stateMgr.terminalFailure.ManifestDigest)
	require.Equal(t, 0, stateMgr.terminalFailure.RollbackAttempts)
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
	require.ErrorContains(t, err, "no previous deployment state")
	require.Empty(t, stateMgr.rollbackName)
	require.Equal(t, testDeploymentMetadata().ManifestDigest, stateMgr.terminalFailure.ManifestDigest)
	require.Equal(t, 0, stateMgr.terminalFailure.RollbackAttempts)
}

func TestDockerDeployerDoesNotStartDeployWhenPreviousStateLoadFailsUnexpectedly(t *testing.T) {
	t.Parallel()

	stateMgr := &fakeStateManager{loadDeployedErr: errors.New("permission denied")}
	notifier := &fakeNotifier{}
	workload := config.Workload{
		Name:          "api",
		ContainerName: "api",
		DeployCommand: "true",
	}

	err := testDockerDeployerWithNotifier(workload, notifier).Deploy(context.Background(), workload, testDeploymentMetadata(), stateMgr)
	require.ErrorContains(t, err, "could not load previous state")
	require.Empty(t, stateMgr.committedName)
	require.Empty(t, stateMgr.rollbackName)
	require.Zero(t, stateMgr.terminalCommits)
	require.Zero(t, notifier.started)
	require.Zero(t, notifier.failed)
	require.Zero(t, notifier.success)
}

func TestDockerDeployerDeployRollsBackToPreviousState(t *testing.T) {
	t.Parallel()

	deployCommand := firstCallFailsThenSucceedsCommand(t)
	previousState := state.DeploymentSnapshot{
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
	require.ErrorContains(t, err, "deployment failed")
	require.ErrorContains(t, err, "rollback succeeded")

	require.Equal(t, "api", stateMgr.rollbackName)
	require.Equal(t, previousState.ManifestDigest, stateMgr.rollback.ManifestDigest)
	require.Equal(t, meta.ManifestDigest, stateMgr.rollback.RollbackFrom)
	require.Equal(t, meta.ManifestDigest, stateMgr.terminalFailure.ManifestDigest)
	require.Equal(t, 1, stateMgr.terminalFailure.RollbackAttempts)
}

func TestDockerDeployerPersistsTerminalFailureWhenDeploymentFailsAndRollbackSucceeds(t *testing.T) {
	t.Parallel()

	previousState := state.DeploymentSnapshot{
		Image:          "ghcr.io/example/api",
		ImageTag:       "v0.9.0",
		ManifestDigest: "sha256:previous",
		GitSHA:         "previous-sha",
		DeployedAt:     time.Now().UTC(),
	}
	stateMgr := &fakeStateManager{deployed: previousState}
	notifier := &fakeNotifier{}
	workload := config.Workload{
		Name:          "api",
		ContainerName: "api",
		DeployCommand: firstCallFailsThenSucceedsCommand(t),
		DeploymentRetry: config.DeploymentRetry{
			MaxRetryForwardDeployment: 1,
			MaxRetryRollback:          3,
			InitialInterval:           config.Duration{Duration: time.Nanosecond},
			MaxInterval:               config.Duration{Duration: time.Nanosecond},
			Multiplier:                1,
		},
	}
	meta := testDeploymentMetadata()

	err := testDockerDeployerWithNotifier(workload, notifier).Deploy(context.Background(), workload, meta, stateMgr)
	require.ErrorContains(t, err, "rollback succeeded")
	require.Equal(t, meta.ManifestDigest, stateMgr.terminalFailure.ManifestDigest)
	require.Equal(t, 1, stateMgr.terminalFailure.ForwardDeploymentAttempts)
	require.Equal(t, 1, stateMgr.terminalFailure.RollbackAttempts)
	require.Equal(t, 1, stateMgr.terminalCommits)
	require.Equal(t, 1, notifier.success)
	require.Equal(t, 1, notifier.failed)
}

func TestDockerDeployerRetriesForwardDeploymentBeforeRollback(t *testing.T) {
	t.Parallel()

	deployCommand := firstCallFailsThenSucceedsCommand(t)
	stateMgr := &fakeStateManager{}
	workload := config.Workload{
		Name:          "api",
		ContainerName: "api",
		DeployCommand: deployCommand,
		DeploymentRetry: config.DeploymentRetry{
			MaxRetryForwardDeployment: 2,
			MaxRetryRollback:          1,
			InitialInterval:           config.Duration{Duration: time.Nanosecond},
			MaxInterval:               config.Duration{Duration: time.Nanosecond},
			Multiplier:                1,
		},
	}

	err := testDockerDeployer(workload).Deploy(context.Background(), workload, testDeploymentMetadata(), stateMgr)
	require.NoError(t, err)
	require.Equal(t, "api", stateMgr.committedName)
	require.Empty(t, stateMgr.rollbackName)
}

func TestDockerDeployerPersistsTerminalFailureWhenDeploymentAndRollbackRetriesExhaust(t *testing.T) {
	t.Parallel()

	previousState := state.DeploymentSnapshot{
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
		DeployCommand: "false",
		DeploymentRetry: config.DeploymentRetry{
			MaxRetryForwardDeployment: 2,
			MaxRetryRollback:          2,
			InitialInterval:           config.Duration{Duration: time.Nanosecond},
			MaxInterval:               config.Duration{Duration: time.Nanosecond},
			Multiplier:                1,
		},
	}
	meta := testDeploymentMetadata()

	err := testDockerDeployer(workload).Deploy(context.Background(), workload, meta, stateMgr)
	require.ErrorContains(t, err, "deployment and rollback failed")
	require.Equal(t, meta.ManifestDigest, stateMgr.terminalFailure.ManifestDigest)
	require.Equal(t, 2, stateMgr.terminalFailure.ForwardDeploymentAttempts)
	require.Equal(t, 2, stateMgr.terminalFailure.RollbackAttempts)
}

func TestDockerDeployerDoesNotRollbackOrPersistTerminalFailureWhenContextCanceled(t *testing.T) {
	t.Parallel()

	previousState := state.DeploymentSnapshot{
		Image:          "ghcr.io/example/api",
		ImageTag:       "v0.9.0",
		ManifestDigest: "sha256:previous",
		GitSHA:         "previous-sha",
		DeployedAt:     time.Now().UTC(),
	}
	stateMgr := &fakeStateManager{deployed: previousState}
	notifier := &fakeNotifier{}
	workload := config.Workload{
		Name:          "api",
		ContainerName: "api",
		DeployCommand: "sleep 10",
		DeploymentRetry: config.DeploymentRetry{
			MaxRetryForwardDeployment: 2,
			MaxRetryRollback:          2,
			InitialInterval:           config.Duration{Duration: time.Nanosecond},
			MaxInterval:               config.Duration{Duration: time.Nanosecond},
			Multiplier:                1,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := testDockerDeployerWithNotifier(workload, notifier).Deploy(ctx, workload, testDeploymentMetadata(), stateMgr)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, stateMgr.rollbackName)
	require.Zero(t, stateMgr.terminalCommits)
	require.Equal(t, 1, notifier.started)
	require.Zero(t, notifier.failed)
	require.Zero(t, notifier.success)
}

func TestDockerDeployerDoesNotRollbackOrPersistTerminalFailureWhenBackoffCanceled(t *testing.T) {
	t.Parallel()

	previousState := state.DeploymentSnapshot{
		Image:          "ghcr.io/example/api",
		ImageTag:       "v0.9.0",
		ManifestDigest: "sha256:previous",
		GitSHA:         "previous-sha",
		DeployedAt:     time.Now().UTC(),
	}
	stateMgr := &fakeStateManager{deployed: previousState}
	notifier := &fakeNotifier{}
	workload := config.Workload{
		Name:          "api",
		ContainerName: "api",
		DeployCommand: "false",
		DeploymentRetry: config.DeploymentRetry{
			MaxRetryForwardDeployment: 2,
			MaxRetryRollback:          2,
			InitialInterval:           config.Duration{Duration: time.Hour},
			MaxInterval:               config.Duration{Duration: time.Hour},
			Multiplier:                1,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	err := testDockerDeployerWithNotifier(workload, notifier).Deploy(ctx, workload, testDeploymentMetadata(), stateMgr)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Empty(t, stateMgr.rollbackName)
	require.Zero(t, stateMgr.terminalCommits)
	require.Equal(t, 1, notifier.started)
	require.Zero(t, notifier.failed)
	require.Zero(t, notifier.success)
}

func TestDockerDeployerCommitDeployedFailureReturnsErrorWithoutSuccessNotification(t *testing.T) {
	t.Parallel()

	stateMgr := &fakeStateManager{commitErr: errors.New("disk full")}
	notifier := &fakeNotifier{}
	workload := config.Workload{
		Name:          "api",
		ContainerName: "api",
		DeployCommand: "true",
	}

	err := testDockerDeployerWithNotifier(workload, notifier).Deploy(context.Background(), workload, testDeploymentMetadata(), stateMgr)
	require.ErrorContains(t, err, "commit deployment state")
	require.Equal(t, "api", stateMgr.committedName)
	require.Zero(t, notifier.success)
	require.Equal(t, 1, notifier.failed)
	require.Zero(t, stateMgr.terminalCommits)
}

func TestDockerDeployerCommitRollbackFailureReturnsErrorAndPersistsTerminalFailure(t *testing.T) {
	t.Parallel()

	previousState := state.DeploymentSnapshot{
		Image:          "ghcr.io/example/api",
		ImageTag:       "v0.9.0",
		ManifestDigest: "sha256:previous",
		GitSHA:         "previous-sha",
		DeployedAt:     time.Now().UTC(),
	}
	stateMgr := &fakeStateManager{
		deployed:    previousState,
		rollbackErr: errors.New("state write failed"),
	}
	notifier := &fakeNotifier{}
	workload := config.Workload{
		Name:          "api",
		ContainerName: "api",
		DeployCommand: firstCallFailsThenSucceedsCommand(t),
	}
	meta := testDeploymentMetadata()

	err := testDockerDeployerWithNotifier(workload, notifier).Deploy(context.Background(), workload, meta, stateMgr)
	require.ErrorContains(t, err, "deployment and rollback failed")
	require.ErrorContains(t, err, "commit rollback state")
	require.Equal(t, "api", stateMgr.rollbackName)
	require.Equal(t, meta.ManifestDigest, stateMgr.terminalFailure.ManifestDigest)
	require.Zero(t, notifier.success)
	require.Equal(t, 2, notifier.failed)
}

func TestDockerDeployerRetriesRollbackAttempts(t *testing.T) {
	t.Parallel()

	previousState := state.DeploymentSnapshot{
		Image:          "ghcr.io/example/api",
		ImageTag:       "v0.9.0",
		ManifestDigest: "sha256:previous",
		GitSHA:         "previous-sha",
		DeployedAt:     time.Now().UTC(),
	}
	stateMgr := &fakeStateManager{
		deployed:    previousState,
		rollbackErr: errors.New("state write failed"),
	}
	workload := config.Workload{
		Name:          "api",
		ContainerName: "api",
		DeployCommand: firstCallFailsThenSucceedsCommand(t),
		DeploymentRetry: config.DeploymentRetry{
			MaxRetryForwardDeployment: 1,
			MaxRetryRollback:          3,
			InitialInterval:           config.Duration{Duration: time.Nanosecond},
			MaxInterval:               config.Duration{Duration: time.Nanosecond},
			Multiplier:                1,
		},
	}

	err := testDockerDeployer(workload).Deploy(context.Background(), workload, testDeploymentMetadata(), stateMgr)
	require.ErrorContains(t, err, "deployment and rollback failed")
	require.Equal(t, 3, stateMgr.rollbackCommits)
	require.Equal(t, 3, stateMgr.terminalFailure.RollbackAttempts)
}

func TestRetryDelayClampsLargeMultiplierAndJitterToMaxInterval(t *testing.T) {
	t.Parallel()

	policy := config.DeploymentRetry{
		InitialInterval: config.Duration{Duration: time.Second},
		MaxInterval:     config.Duration{Duration: 10 * time.Second},
		Multiplier:      1e308,
	}

	for range 100 {
		delay := retryDelay(policy, 2)
		require.Positive(t, delay)
		require.LessOrEqual(t, delay, policy.MaxInterval.Duration)
	}
}

func testDockerDeployer(workload config.Workload) *dockerDeployer {
	return testDockerDeployerWithNotifier(workload, notifier.Noop{})
}

func testDockerDeployerWithNotifier(workload config.Workload, n notifier.Notifier) *dockerDeployer {
	return NewDockerDeployer(
		workload,
		n,
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
