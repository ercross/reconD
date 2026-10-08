// Package reconciler implements the core control loop for a single workload.
package reconciler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ercross/reconD/config"
	"github.com/ercross/reconD/deployment"
	"github.com/ercross/reconD/git_provider"
	"github.com/ercross/reconD/logger"
	"github.com/ercross/reconD/notifier"
	"github.com/ercross/reconD/state"
)

type Reconciler struct {
	workload     config.Workload
	pollInterval time.Duration
	gitProvider  git_provider.GitProvider
	deployer     deployment.Deployer
	stateMgr     state.Manager
	log          *slog.Logger
}

func New(
	workload config.Workload,
	gitProvider git_provider.GitProvider,
	stateMgr state.Manager,
	alertManager notifier.Notifier,
	log *slog.Logger,
) *Reconciler {
	workloadLog := logger.WithContainerName(log, workload.ContainerName)
	workloadLog = workloadLog.With("workload", workload.Name)
	d := deployment.NewDockerDeployer(workload, alertManager, workloadLog)

	return &Reconciler{
		workload:     workload,
		pollInterval: workload.CheckInterval.Duration,
		gitProvider:  gitProvider,
		deployer:     d,
		stateMgr:     stateMgr,
		log:          workloadLog,
	}
}

func (r *Reconciler) StartPeriodicReconciliation(ctx context.Context) {
	r.log.Info("reconciler started", "poll_interval", r.pollInterval)
	r.reconcileOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			r.log.Info("reconciler shutting down", "reason", ctx.Err())
			return
		case <-time.After(r.pollInterval):
			r.reconcileOnce(ctx)
		}
	}
}

func (r *Reconciler) reconcileOnce(ctx context.Context) {
	start := time.Now()

	r.log.Debug("reconcile cycle started", "phase", logger.PhasePoll)

	desiredState, err := r.fetchDesiredState(ctx)
	if err != nil {
		if errors.Is(err, git_provider.ErrNoDeploymentMetaFound) {
			r.log.Info("no deployment metadata found",
				"phase", logger.PhasePoll,
				"release_prefix", r.workload.ReleasePrefix,
			)
			return
		}
		r.log.Error("failed to fetch desired state",
			"phase", logger.PhasePoll,
			"error", err,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return
	}

	currentState, err := r.stateMgr.LoadDeployed(r.workload.Name)
	if err != nil && !state.IsNotFound(err) {
		r.log.Error("failed to load deployed state",
			"phase", logger.PhaseDrift,
			"error", err,
		)
		return
	}

	hasDrift, driftReason, err := r.hasDrift(ctx, currentState, desiredState)
	if err != nil {
		r.log.Error("failed to observe runtime state",
			"phase", logger.PhaseDrift,
			"error", err,
		)
		return
	}
	if !hasDrift {
		r.log.Debug("no drift detected",
			"phase", logger.PhaseNoop,
			"digest", desiredState.ManifestDigest,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return
	}

	r.log.Info("drift detected; deployment required",
		"phase", logger.PhaseDrift,
		"drift_reason", driftReason,
	)

	if err = r.deployer.Deploy(ctx, r.workload, desiredState, r.stateMgr); err != nil {
		r.log.Error("deployment failed",
			"error", err,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return
	}

	r.log.Info("reconcile cycle completed successfully",
		"digest", desiredState.ManifestDigest,
		"duration_ms", time.Since(start).Milliseconds(),
	)
}

func (r *Reconciler) fetchDesiredState(ctx context.Context) (git_provider.DeploymentMetadata, error) {
	if r.gitProvider == nil {
		return git_provider.DeploymentMetadata{}, fmt.Errorf("workload %q has no git provider", r.workload.Name)
	}
	meta, err := r.gitProvider.FetchLatestDeploymentMetadata(ctx, r.workload.ReleasePrefix)
	if err != nil {
		return meta, fmt.Errorf("failed to fetch metadata: %w", err)
	}
	return meta, r.validateDesiredState(meta)
}

func (r *Reconciler) validateDesiredState(meta git_provider.DeploymentMetadata) error {
	if meta.Image == "" {
		return fmt.Errorf("deployment metadata image is required")
	}
	if meta.ManifestDigest == "" {
		return fmt.Errorf("deployment metadata manifest_digest is required")
	}
	return nil
}

func (r *Reconciler) hasDrift(ctx context.Context, actual state.DeploymentState, desired git_provider.DeploymentMetadata) (bool, string, error) {
	if actual.ManifestDigest == "" {
		return true, "no current deployment state", nil
	}
	if actual.ManifestDigest != desired.ManifestDigest {
		return true, fmt.Sprintf("current digest [%s] != desired digest [%s]", actual.ManifestDigest, desired.ManifestDigest), nil
	}

	runtimeState, err := r.deployer.CurrentRuntimeState(ctx, r.workload)
	if err != nil {
		return false, "", err
	}
	if !runtimeState.Running {

		// if container not running, no drift.
		// container lifecycle management is out of scope for this project
		return false, "", nil
	}

	if runtimeState.ContainerManifestDigest == "" {
		return true, "runtime container manifest digest is unavailable", nil
	}

	if actual.ManifestDigest != runtimeState.ContainerManifestDigest {
		return true, fmt.Sprintf("current state digest [%s] != runtime container digest [%s]", actual.ManifestDigest, runtimeState.ContainerManifestDigest), nil
	}

	return false, "", nil
}
