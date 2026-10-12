package reconciler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/ercross/reconD/config"
	"github.com/ercross/reconD/deployment"
	"github.com/ercross/reconD/git_provider"
	"github.com/ercross/reconD/state"
	"github.com/stretchr/testify/require"
)

type fakeGitProvider struct {
	meta git_provider.DeploymentMetadata
	err  error
}

func (p fakeGitProvider) FetchLatestDeploymentMetadata(context.Context, string) (git_provider.DeploymentMetadata, error) {
	return p.meta, p.err
}

type fakeDeployer struct {
	deployCalls int
}

func (d *fakeDeployer) PruneDanglingImages(context.Context, map[string]string) error {
	return nil
}

func (d *fakeDeployer) CurrentRuntimeState(context.Context, config.Workload) (deployment.RuntimeState, error) {
	return deployment.RuntimeState{}, nil
}

func (d *fakeDeployer) Deploy(context.Context, config.Workload, git_provider.DeploymentMetadata, state.Manager) error {
	d.deployCalls++
	return nil
}

type fakeStateManager struct {
	loadDeployedErr     error
	deployed            state.DeploymentSnapshot
	loadTerminalFailure state.TerminalFailureState
	loadTerminalErr     error
	clearFailureCalls   int
}

func (m *fakeStateManager) LoadDeployed(string) (state.DeploymentSnapshot, error) {
	return m.deployed, m.loadDeployedErr
}

func (m *fakeStateManager) LoadPrevious(string) (state.DeploymentSnapshot, error) {
	return state.DeploymentSnapshot{}, nil
}

func (m *fakeStateManager) LoadTerminalFailure(string) (state.TerminalFailureState, error) {
	if m.loadTerminalErr != nil {
		return state.TerminalFailureState{}, m.loadTerminalErr
	}
	if m.loadTerminalFailure.ManifestDigest == "" {
		return state.TerminalFailureState{}, state.ErrDeploymentStateNotFound
	}
	return m.loadTerminalFailure, nil
}

func (m *fakeStateManager) CommitDeployed(string, state.DeploymentSnapshot) error {
	return nil
}

func (m *fakeStateManager) CommitRollback(string, state.DeploymentSnapshot) error {
	return nil
}

func (m *fakeStateManager) CommitTerminalFailure(string, state.TerminalFailureState) error {
	return nil
}

func (m *fakeStateManager) ClearTerminalFailure(string) error {
	m.clearFailureCalls++
	return nil
}

func TestReconcileDeploysWhenStateFileIsMissing(t *testing.T) {
	t.Parallel()

	deployer := &fakeDeployer{}
	r := testReconciler(state.ErrFileNotFound, deployer)

	r.reconcileOnce(context.Background())

	require.Equal(t, 1, deployer.deployCalls)
}

func TestReconcileDeploysWhenDeploymentStateIsMissing(t *testing.T) {
	t.Parallel()

	deployer := &fakeDeployer{}
	r := testReconciler(state.ErrDeploymentStateNotFound, deployer)

	r.reconcileOnce(context.Background())

	require.Equal(t, 1, deployer.deployCalls)
}

func TestReconcileSkipsDeployWhenStateLoadFailsUnexpectedly(t *testing.T) {
	t.Parallel()

	deployer := &fakeDeployer{}
	r := testReconciler(errors.New("permission denied"), deployer)

	r.reconcileOnce(context.Background())

	require.Zero(t, deployer.deployCalls)
}

func TestReconcileSkipsDeployWhenTerminalFailureLoadFailsUnexpectedly(t *testing.T) {
	t.Parallel()

	deployer := &fakeDeployer{}
	r := testReconciler(state.ErrDeploymentStateNotFound, deployer)
	r.stateMgr = &fakeStateManager{
		loadDeployedErr: state.ErrDeploymentStateNotFound,
		loadTerminalErr: errors.New("permission denied"),
	}

	r.reconcileOnce(context.Background())

	require.Zero(t, deployer.deployCalls)
}

func TestReconcileSkipsDeployWhenTerminalFailureMatchesDesiredDigest(t *testing.T) {
	t.Parallel()

	deployer := &fakeDeployer{}
	r := testReconciler(state.ErrDeploymentStateNotFound, deployer)
	r.stateMgr = &fakeStateManager{
		loadDeployedErr: state.ErrDeploymentStateNotFound,
		loadTerminalFailure: state.TerminalFailureState{
			ManifestDigest: "sha256:desired",
		},
	}

	r.reconcileOnce(context.Background())

	require.Zero(t, deployer.deployCalls)
}

func TestReconcileDeploysWhenTerminalFailureIsForDifferentDigest(t *testing.T) {
	t.Parallel()

	deployer := &fakeDeployer{}
	r := testReconciler(state.ErrDeploymentStateNotFound, deployer)
	stateMgr := &fakeStateManager{
		loadDeployedErr: state.ErrDeploymentStateNotFound,
		loadTerminalFailure: state.TerminalFailureState{
			ManifestDigest: "sha256:old",
		},
	}
	r.stateMgr = stateMgr

	r.reconcileOnce(context.Background())

	require.Equal(t, 1, deployer.deployCalls)
	require.Equal(t, 1, stateMgr.clearFailureCalls)
}

func TestReconcileClearsMatchingTerminalFailureWhenDesiredIsAlreadyDeployed(t *testing.T) {
	t.Parallel()

	deployer := &fakeDeployer{}
	r := testReconciler(nil, deployer)
	stateMgr := &fakeStateManager{
		deployed: state.DeploymentSnapshot{
			ManifestDigest: "sha256:desired",
		},
		loadTerminalFailure: state.TerminalFailureState{
			ManifestDigest: "sha256:desired",
		},
	}
	r.stateMgr = stateMgr

	r.reconcileOnce(context.Background())

	require.Zero(t, deployer.deployCalls)
	require.Equal(t, 1, stateMgr.clearFailureCalls)
}

func TestReconcileClearsStaleTerminalFailureWhenDesiredIsAlreadyDeployed(t *testing.T) {
	t.Parallel()

	deployer := &fakeDeployer{}
	r := testReconciler(nil, deployer)
	stateMgr := &fakeStateManager{
		deployed: state.DeploymentSnapshot{
			ManifestDigest: "sha256:desired",
		},
		loadTerminalFailure: state.TerminalFailureState{
			ManifestDigest: "sha256:old",
		},
	}
	r.stateMgr = stateMgr

	r.reconcileOnce(context.Background())

	require.Zero(t, deployer.deployCalls)
	require.Equal(t, 1, stateMgr.clearFailureCalls)
}

func testReconciler(loadDeployedErr error, deployer *fakeDeployer) *Reconciler {
	return &Reconciler{
		workload: config.Workload{
			Name:          "api",
			ContainerName: "api",
			CheckInterval: config.Duration{Duration: time.Minute},
		},
		pollInterval: time.Minute,
		gitProvider: fakeGitProvider{meta: git_provider.DeploymentMetadata{
			Image:          "ghcr.io/example/api",
			ManifestDigest: "sha256:desired",
		}},
		deployer: deployer,
		stateMgr: &fakeStateManager{loadDeployedErr: loadDeployedErr},
		log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}
