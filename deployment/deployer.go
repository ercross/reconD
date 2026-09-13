// Package deployment applies desired workload versions to the local container
// runtime and handles rollback when deployment fails.
package deployment

import (
	"context"
	"errors"

	"github.com/ercross/reconD/config"
	"github.com/ercross/reconD/git_provider"
	"github.com/ercross/reconD/state"
)

type Deployer interface {
	PruneDanglingImages(ctx context.Context, targetLabels map[string]string) error
	CurrentRuntimeState(ctx context.Context, workload config.Workload) (RuntimeState, error)
	Deploy(ctx context.Context, workload config.Workload, meta git_provider.DeploymentMetadata, stateMgr state.Manager) error
}

var (
	errImagePullFailed          = errors.New("error encountered during image pull")
	errDeploymentStrategyFailed = errors.New("deployment strategy failed")
	errDeployCommandFailed      = errors.New("failed to run deploy command")
	errHealthCheckFailed        = errors.New("new deployment failed health check")
)

var errorsThatCanTriggerRollback = []error{
	errImagePullFailed, errDeployCommandFailed, errHealthCheckFailed,
}

// RuntimeState is the reconciler's view of the currently running workload.
type RuntimeState struct {
	Running                 bool
	ContainerManifestDigest string
	Reason                  string
}
