// Package reconciler implements the core control loop for a single environment.
//
// The reconciler is the heart of the deployment agent. It continuously:
//  1. Polls GitHub Releases for the desired state of the environment
//  2. Compares the desired state (remote manifest digest) against the actual
//     state (locally persisted deployed digest)
//  3. If they differ (drift detected), delegates to the Orchestrator to converge
//  4. Sleeps for poll_interval and repeats
//
// Each environment runs its own independent reconciler goroutine.
// A failure in the dev reconciler cannot affect the production reconciler.
//
// Design: Reconcile-then-sleep (not ticker) so a slow deployment doesn't
// cause reconcile loops to overlap.
package reconciler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ercross/reconD/config"
	"github.com/ercross/reconD/deployment_orchestrator"
	"github.com/ercross/reconD/git_provider"
	"github.com/ercross/reconD/logger"
	"github.com/ercross/reconD/notifier"
	"github.com/ercross/reconD/state"
)

// Reconciler manages the continuous reconciliation loop for one environment.
type Reconciler struct {
	target             config.Service
	pollInterval       time.Duration
	gitProvider        git_provider.GitProvider
	deployOrchestrator deployment_orchestrator.DeploymentOrchestrator
	stateMgr           state.Manager
	log                *slog.Logger
}

// New creates a Reconciler for one environment.
func New(
	target config.Service,
	pollInterval time.Duration,
	gitProvider git_provider.GitProvider,
	stateMgr state.Manager,
	alertManager notifier.Notifier,
	log *slog.Logger,
) *Reconciler {
	envLog := logger.WithContainerName(log, target.ContainerName)
	envLog = envLog.With("service", target.Name)
	d := deployment_orchestrator.NewDockerOrchestrator(target, alertManager, envLog)

	return &Reconciler{
		target:             target,
		pollInterval:       pollInterval,
		gitProvider:        gitProvider,
		deployOrchestrator: d,
		stateMgr:           stateMgr,
		log:                envLog,
	}
}

// StartPeriodicReconciliation starts the reconciliation loop and blocks until ctx is cancelled.
// It is safe to run multiple Reconcilers concurrently in separate goroutines.
func (r *Reconciler) StartPeriodicReconciliation(ctx context.Context) {
	r.log.Info("reconciler started", "poll_interval", r.pollInterval)

	// Run an immediate reconcile on startup rather than waiting for the first
	// tick. This means deployments that happened while the agent was down are
	// applied within seconds of agent start.
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

// reconcileOnce runs a single poll-compare-converge cycle.
// It is the unit of work: safe to call repeatedly, never panics.
func (r *Reconciler) reconcileOnce(ctx context.Context) {
	start := time.Now()

	r.log.Debug("reconcile cycle started", "phase", logger.PhasePoll)

	// --- Step 1: Poll desired state from GitHub Releases ---
	desiredState, err := r.fetchDesiredState(ctx)
	if err != nil {
		if errors.Is(err, git_provider.ErrNoDeploymentMetaFound) {
			r.log.Info("no release found for this container",
				"phase", logger.PhasePoll,
				"release_prefix", r.target.Reconciler.ReleasePrefix,
			)
			return
		}
		r.log.Error("failed to fetch desired state",
			"phase", logger.PhasePoll,
			"error", err,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		// Transient failure — will retry on next poll.
		return
	}

	r.log.Debug("desired state fetched",
		"phase", logger.PhasePoll,
		"digest", desiredState.ManifestDigest,
		"git_sha", desiredState.GitSHA,
	)

	// --- Step 2: Load currentState (deployed) state ---
	currentState, err := r.stateMgr.LoadDeployed()
	// if fileNotFound error, that usually means no previous deployed state and this is likely the first run,
	// drift in that case
	if err != nil && !errors.Is(err, state.ErrFileNotFound) {
		r.log.Error("failed to load deployed state",
			"phase", logger.PhaseDrift,
			"error", err,
		)
		return
	}

	// --- Step 3: Drift detection ---
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

	r.log.Info("drift detected — deployment required",
		"phase", logger.PhaseDrift,
		"drift_reason", driftReason,
	)

	// --- Step 4: Deploy ---
	if err = r.deployOrchestrator.Deploy(ctx, r.target, desiredState, r.stateMgr); err != nil {
		r.log.Error("deployment failed",
			"error", err,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		// Error is already logged with full detail by the deployment_engine.
		// Next reconcile cycle will retry (or find the drift resolved
		// if a rollback was successful).
		return
	}

	r.log.Info("reconcile cycle completed successfully",
		"digest", desiredState.ManifestDigest,
		"duration_ms", time.Since(start).Milliseconds(),
	)
}

// fetchDesiredState retrieves the latest deployment metadata for this environment.
func (r *Reconciler) fetchDesiredState(ctx context.Context) (meta git_provider.DeploymentMetadata, err error) {
	if r.target.Reconciler == nil {
		return meta, fmt.Errorf("target service %q has no reconciler", r.target.Name)
	}
	meta, err = r.gitProvider.FetchLatestForEnvironment(ctx, r.target.Reconciler.ReleasePrefix, r.env.Name)
	if err != nil {
		return meta, fmt.Errorf("failed to fetch metadata: %w", err)
	}
	return meta, nil
}

// hasDrift reports whether the deployed state differs from the desired state.
//
// Drift is detected by comparing manifest digests. Image tags are mutable
// and cannot be trusted for this purpose — the same tag (e.g. :dev-latest)
// can point to different images over time.
//
// If actual is nil (no state persisted), drift always exists (first deploy).
func (r *Reconciler) hasDrift(ctx context.Context, actual state.DeploymentState, desired git_provider.DeploymentMetadata) (hasDrift bool, driftReason string, err error) {
	// GitHub says desired digest = sha-new
	// state file says deployed digest = sha-old
	// => drift detected
	if actual.ManifestDigest != desired.ManifestDigest {
		return true, fmt.Sprintf("current state manifest digest [%s] != desired manifest digest [%s]", actual.ManifestDigest, desired.ManifestDigest), nil
	}

	// Check runtime drift if container is running

	// TODO this line triggers false redeployment, captures wrong runtimeState.ManifestDigest
	// modify the function so it captures the correct manifestDigest and compare if desired digest is runnning
	//if runtimeState.ManifestDigest != desired.ManifestDigest {
	//	return true, fmt.Sprintf("runtime manifest digest [%s] != desired manifest digest [%s]", runtimeState.ManifestDigest, desired.ManifestDigest), nil
	//}

	return false, "", nil
}

// deployedDigest safely returns the digest of the current state.
// Returns "<none>" if no state exists (for structured log fields).
func deployedDigest(s state.DeploymentState) string {
	if s.ManifestDigest == "" {
		return "<none>"
	}
	return s.ManifestDigest
}
