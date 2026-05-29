// Package deployment_orchestrator implements the deployment execution layer.
// deployment_engine.go implements the deployment deployment_engine.
// It coordinates the end-to-end deployment sequence:
//
//  1. Pull updated image
//  2. Run database migrations
//  3. Restart application
//  4. Health check
//  5. Commit state (or rollback on failure)
//
// This is intentionally separate from the reconciler (which detects drift)
// and the compose executor (which runs commands). The deployment_engine owns the
// deployment lifecycle and rollback logic.
package deployment_orchestrator

import (
	"context"
	"errors"

	"github.com/ercross/reconD/config"
	"github.com/ercross/reconD/git_provider"
	"github.com/ercross/reconD/state"
)

type DeploymentOrchestrator interface {
	PullImage(ctx context.Context, service string) error

	// PruneDanglingImages removes dangling images — images with no tag and no
	// container referencing them. These accumulate after force-recreate deploys
	// where the old image layer is superseded by a new pull of the same tag.
	//
	// This is preferable to targeted RemoveImages because:
	//   - No need to track which refs to remove
	//   - Safe by definition: dangling = unreferenced, so nothing running can be affected
	//   - Handles the mutable-tag case (:dev-latest) where the old layers have
	//     no tag anymore after the new pull, making them unaddressable by ref anyway
	PruneDanglingImages(ctx context.Context, targetLabels map[string]string) error

	// CurrentRuntimeState implements RuntimeObserver.
	//
	// Uses `docker compose ps --format json` to determine whether all expected
	// services exist and are in a running state.
	CurrentRuntimeState(ctx context.Context, services []string) (RuntimeState, error)
	Deploy(ctx context.Context, target config.Service, meta git_provider.DeploymentMetadata, stateMgr state.Manager) error
}

var (
	errMigrationFailed   = errors.New("error encountered during migration")
	errImagePullFailed   = errors.New("error encountered during image pull")
	errRestartFailed     = errors.New("failed to restart service")
	errHealthCheckFailed = errors.New("new deployment failed health check")
	errComposeEnvFailed  = errors.New("failed to update compose env file")
)

var errorsThatCanTriggerRollback = []error{
	errImagePullFailed, errRestartFailed, errHealthCheckFailed,
}

// RuntimeState is the reconciler's view of the currently running application.
type RuntimeState struct {
	Running        bool
	ManifestDigest string
	Reason         string
}
