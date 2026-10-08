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
	loadDeployedErr error
	deployed        state.DeploymentState
}

func (m fakeStateManager) LoadDeployed(string) (state.DeploymentState, error) {
	return m.deployed, m.loadDeployedErr
}

func (m fakeStateManager) LoadPrevious(string) (state.DeploymentState, error) {
	return state.DeploymentState{}, nil
}

func (m fakeStateManager) CommitDeployed(string, state.DeploymentState) error {
	return nil
}

func (m fakeStateManager) CommitRollback(string, state.DeploymentState) error {
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
		stateMgr: fakeStateManager{loadDeployedErr: loadDeployedErr},
		log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}
