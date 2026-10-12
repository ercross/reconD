package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ercross/reconD/config"
	"github.com/ercross/reconD/git_provider"
	"github.com/ercross/reconD/health"
	"github.com/ercross/reconD/logger"
	"github.com/ercross/reconD/notifier"
	"github.com/ercross/reconD/state"
)

type dockerDeployer struct {
	workload config.Workload
	logger   *slog.Logger
	notifier notifier.Notifier
	strategy MetadataApplicationStrategy
}

func NewDockerDeployer(
	workload config.Workload,
	notifier notifier.Notifier,
	log *slog.Logger,
) Deployer {
	strategy := NewStrategy(workload.Strategy, log)
	return &dockerDeployer{
		workload: workload,
		logger:   log,
		notifier: notifier,
		strategy: strategy,
	}
}

func (d *dockerDeployer) PruneDanglingImages(ctx context.Context, targetLabels map[string]string) error {
	args := []string{"image", "prune", "-a", "--force"}

	for key, value := range targetLabels {
		filterExpr := fmt.Sprintf("label=%s=%s", key, value)
		args = append(args, "--filter", filterExpr)
	}

	return d.runDocker(ctx, "image_prune", args...)
}

func (d *dockerDeployer) CurrentRuntimeState(ctx context.Context, workload config.Workload) (RuntimeState, error) {
	containers, stderr, err := d.runDockerOutput(ctx, "runtime_inspect", "inspect", workload.ContainerName)
	if err != nil {
		if isNoSuchContainer(stderr) {
			return RuntimeState{Running: false, Reason: "container does not exist"}, nil
		}
		return RuntimeState{}, fmt.Errorf("inspect container %q: %w", workload.ContainerName, err)
	}

	var inspected []dockerContainerInspect
	if err := json.Unmarshal(containers, &inspected); err != nil {
		return RuntimeState{}, fmt.Errorf("parse container inspect output: %w", err)
	}
	if len(inspected) == 0 {
		return RuntimeState{Running: false, Reason: "container does not exist"}, nil
	}
	if !inspected[0].State.Running {
		return RuntimeState{Running: false, Reason: "container is not running"}, nil
	}

	manifestDigest, err := d.containerManifestDigest(ctx, inspected[0])
	if err != nil {
		return RuntimeState{}, err
	}

	return RuntimeState{
		Running:                 true,
		ContainerManifestDigest: manifestDigest,
	}, nil
}

func (d *dockerDeployer) runDocker(ctx context.Context, phase string, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	d.logger.Debug("executing docker",
		"phase", phase,
		"args", strings.Join(args, " "),
	)

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	if err != nil {
		d.logger.Error("docker command failed",
			"phase", phase,
			"duration_ms", duration.Milliseconds(),
			"stdout", strings.TrimSpace(stdout.String()),
			"stderr", strings.TrimSpace(stderr.String()),
			"error", err,
		)
		return fmt.Errorf("docker %s failed: %w\nstderr: %s",
			phase, err, strings.TrimSpace(stderr.String()))
	}

	d.logger.Info("docker command succeeded",
		"phase", phase,
		"duration_ms", duration.Milliseconds(),
	)
	return nil
}

func (d *dockerDeployer) runDockerOutput(ctx context.Context, phase string, args ...string) ([]byte, string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	d.logger.Debug("executing docker",
		"phase", phase,
		"args", strings.Join(args, " "),
	)

	err := cmd.Run()
	if err != nil {
		return stdout.Bytes(), strings.TrimSpace(stderr.String()), fmt.Errorf("docker %s failed: %w\nstderr: %s", phase, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), strings.TrimSpace(stderr.String()), nil
}

func (d *dockerDeployer) Deploy(ctx context.Context, workload config.Workload, meta git_provider.DeploymentMetadata, stateMgr state.Manager) error {
	d.logger.Info("starting deployment",
		"phase", logger.PhasePull,
		"workload", workload.Name,
		"digest", meta.ManifestDigest,
		"git_sha", meta.GitSHA,
	)

	previousState, err := stateMgr.LoadDeployed(workload.Name)
	if err != nil && !state.IsNotFound(err) {
		return fmt.Errorf("could not load previous state: %w", err)
	}
	noPreviousState := state.IsNotFound(err)

	d.notifier.NotifyOnNewDeploymentStarted(workload.Name, meta)

	newState, err := d.deployWithRetry(ctx, workload, meta, stateMgr)
	if err == nil {
		d.notifier.NotifyOnDeploymentSuccess(workload.Name, newState)
		if err = d.PruneDanglingImages(ctx, workload.Labels); err != nil {
			d.logger.Error("failed to prune dangling images", "phase", logger.PhaseCommit, "workload", workload.Name, "error", err)
		}
		return nil
	}

	d.logger.Error("forward deployment failed", "workload", workload.Name, "error", err)
	if contextIsDone(err) {
		return err
	}
	if !errorShouldTriggerRollback(err) {
		d.notifier.NotifyOnDeploymentFailed(workload.Name, meta, err)
		return fmt.Errorf("deployment failed but no rollback required: %w", err)
	}

	if noPreviousState {
		d.logger.Error("no previous state available for rollback", "phase", logger.PhaseRollback)
		d.notifier.NotifyOnDeploymentFailed(workload.Name, meta, err)
		if terminalErr := d.commitTerminalFailureOnNoPreviousStateAvailable(stateMgr, workload, meta, err); terminalErr != nil {
			return fmt.Errorf("deployment failed; failed to persist no-previous-state terminal failure: %v", terminalErr)
		}
		return fmt.Errorf("deployment failed: no previous deployment state; no rollback attempted")
	}

	d.notifier.NotifyOnDeploymentFailed(workload.Name, meta, err)
	rollbackState, rollbackAttempts, rollbackErr := d.rollbackWithRetry(ctx, stateMgr, workload, previousState, meta, err.Error())
	if rollbackErr == nil {
		d.notifier.NotifyOnDeploymentSuccess(workload.Name, rollbackState)
		if terminalErr := d.commitTerminalFailure(stateMgr, workload, meta, err, nil, rollbackAttempts); terminalErr != nil {
			return fmt.Errorf("deployment failed and rollback succeeded; additionally failed to persist terminal failure: %v", terminalErr)
		}
		return fmt.Errorf("deployment failed after %d forward attempts; rollback succeeded after %d attempts", deploymentRetryPolicy(workload).MaxRetryForwardDeployment, rollbackAttempts)
	}
	if contextIsDone(rollbackErr) {
		return rollbackErr
	}

	d.notifier.NotifyOnDeploymentFailed(workload.Name, meta, fmt.Errorf("ROLLBACK FAILED: %w", rollbackErr))
	if terminalErr := d.commitTerminalFailure(stateMgr, workload, meta, err, rollbackErr, rollbackAttempts); terminalErr != nil {
		return fmt.Errorf("deployment and rollback failed: %w; additionally failed to persist terminal failure: %v", rollbackErr, terminalErr)
	}
	return fmt.Errorf("deployment and rollback failed: %w", rollbackErr)
}

func (d *dockerDeployer) deployWithRetry(ctx context.Context, workload config.Workload, meta git_provider.DeploymentMetadata, stateMgr state.Manager) (state.DeploymentSnapshot, error) {
	policy := deploymentRetryPolicy(workload)
	var lastErr error

	for attempt := 1; attempt <= policy.MaxRetryForwardDeployment; attempt++ {
		d.logger.Info("running deployment attempt",
			"phase", logger.PhaseRestart,
			"workload", workload.Name,
			"attempt", attempt,
			"max_attempts", policy.MaxRetryForwardDeployment,
		)

		newState, err := d.deploy(ctx, workload, meta, stateMgr)
		if err == nil {
			return newState, nil
		}
		if contextIsDone(err) {
			return state.DeploymentSnapshot{}, err
		}
		lastErr = err
		if !errorShouldTriggerRollback(err) || attempt == policy.MaxRetryForwardDeployment {
			break
		}
		if err := d.sleepBeforeRetry(ctx, policy, attempt, "forward_deployment", workload.Name, err); err != nil {
			return state.DeploymentSnapshot{}, err
		}
	}

	return state.DeploymentSnapshot{}, lastErr
}

func (d *dockerDeployer) rollbackWithRetry(ctx context.Context, stateMgr state.Manager, workload config.Workload, previousState state.DeploymentSnapshot, failedMeta git_provider.DeploymentMetadata, reason string) (state.DeploymentSnapshot, int, error) {
	policy := deploymentRetryPolicy(workload)
	var lastErr error
	attempts := 0

	for attempt := 1; attempt <= policy.MaxRetryRollback; attempt++ {
		attempts = attempt
		d.logger.Info("running rollback attempt",
			"phase", logger.PhaseRollback,
			"workload", workload.Name,
			"attempt", attempt,
			"max_attempts", policy.MaxRetryRollback,
		)

		rollbackState, err := d.rollback(ctx, stateMgr, workload, previousState, failedMeta.ManifestDigest, reason)
		if err == nil {
			return rollbackState, attempts, nil
		}
		if contextIsDone(err) {
			return state.DeploymentSnapshot{}, attempts, err
		}
		lastErr = err
		if attempt == policy.MaxRetryRollback {
			break
		}
		if err := d.sleepBeforeRetry(ctx, policy, attempt, "rollback", workload.Name, err); err != nil {
			return state.DeploymentSnapshot{}, attempts, err
		}
	}

	return state.DeploymentSnapshot{}, attempts, lastErr
}

func (d *dockerDeployer) sleepBeforeRetry(ctx context.Context, policy config.DeploymentRetry, failedAttempt int, retryPhase, workloadName string, cause error) error {
	delay := retryDelay(policy, failedAttempt)
	d.logger.Warn("attempt failed; retrying after backoff",
		"phase", retryPhase,
		"workload", workloadName,
		"failed_attempt", failedAttempt,
		"next_backoff_ms", delay.Milliseconds(),
		"error", cause,
	)

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("%s retry backoff canceled: %w", retryPhase, ctx.Err())
	case <-timer.C:
		return nil
	}
}

func retryDelay(policy config.DeploymentRetry, failedAttempt int) time.Duration {
	delay := policy.InitialInterval.Duration
	for i := 1; i < failedAttempt; i++ {
		next := float64(delay) * policy.Multiplier
		if next >= float64(policy.MaxInterval.Duration) {
			delay = policy.MaxInterval.Duration
			break
		}
		delay = time.Duration(next)
		if delay <= 0 {
			delay = policy.MaxInterval.Duration
			break
		}
	}
	delay = min(delay, policy.MaxInterval.Duration)
	if delay <= 0 {
		return 0
	}

	jitterRange := int64(delay / 2)
	if jitterRange <= 0 {
		return delay
	}
	jitter := time.Duration(rand.Int63n(jitterRange))
	return min(delay-delay/4+jitter, policy.MaxInterval.Duration)
}

func (d *dockerDeployer) commitTerminalFailure(stateMgr state.Manager, workload config.Workload, meta git_provider.DeploymentMetadata, deployErr, rollbackErr error, rollbackAttempts int) error {
	policy := deploymentRetryPolicy(workload)
	reason := fmt.Sprintf("deployment failed: %v", deployErr)
	if rollbackErr == nil {
		reason += "; rollback succeeded"
	} else {
		reason += fmt.Sprintf("; rollback failed: %v", rollbackErr)
	}
	failure := state.TerminalFailureState{
		ManifestDigest:            meta.ManifestDigest,
		GitSHA:                    meta.GitSHA,
		ForwardDeploymentAttempts: policy.MaxRetryForwardDeployment,
		RollbackAttempts:          rollbackAttempts,
		Reason:                    reason,
		FailedAt:                  time.Now().UTC(),
	}
	return stateMgr.CommitTerminalFailure(workload.Name, failure)
}

func (d *dockerDeployer) commitTerminalFailureOnNoPreviousStateAvailable(stateMgr state.Manager, workload config.Workload, meta git_provider.DeploymentMetadata, deployErr error) error {
	policy := deploymentRetryPolicy(workload)
	failure := state.TerminalFailureState{
		ManifestDigest:            meta.ManifestDigest,
		GitSHA:                    meta.GitSHA,
		ForwardDeploymentAttempts: policy.MaxRetryForwardDeployment,
		RollbackAttempts:          0,
		Reason:                    fmt.Sprintf("deployment failed: %v, no previous state available, no rollback attempted", deployErr),
		FailedAt:                  time.Now().UTC(),
	}
	return stateMgr.CommitTerminalFailure(workload.Name, failure)
}

func deploymentRetryPolicy(workload config.Workload) config.DeploymentRetry {
	policy := workload.DeploymentRetry
	if policy.MaxRetryForwardDeployment < 1 {
		policy.MaxRetryForwardDeployment = 1
	}
	if policy.MaxRetryRollback < 1 {
		policy.MaxRetryRollback = 1
	}
	if policy.InitialInterval.Duration <= 0 {
		policy.InitialInterval.Duration = time.Second
	}
	if policy.MaxInterval.Duration <= 0 {
		policy.MaxInterval.Duration = 30 * time.Second
	}
	if policy.MaxInterval.Duration < policy.InitialInterval.Duration {
		policy.MaxInterval.Duration = policy.InitialInterval.Duration
	}
	if policy.Multiplier < 1 || policy.Multiplier != policy.Multiplier {
		policy.Multiplier = 2
	}
	return policy
}

func (d *dockerDeployer) deploy(ctx context.Context, workload config.Workload, meta git_provider.DeploymentMetadata, stateMgr state.Manager) (state.DeploymentSnapshot, error) {
	var newState state.DeploymentSnapshot

	if err := d.runStrategy(ctx, workload, meta); err != nil {
		if ctx.Err() != nil {
			return newState, ctx.Err()
		}
		return newState, errors.Join(errDeploymentStrategyFailed, err)
	}

	d.logger.Info("running deploy command", "phase", logger.PhaseRestart, "workload", workload.Name)
	if err := d.runDeployCommand(ctx, workload, meta); err != nil {
		if ctx.Err() != nil {
			return newState, ctx.Err()
		}
		return newState, errors.Join(errDeployCommandFailed, err)
	}

	if err := d.waitHealthy(ctx, workload); err != nil {
		if ctx.Err() != nil {
			return newState, ctx.Err()
		}
		return newState, errors.Join(errHealthCheckFailed, err)
	}

	newState = state.DeploymentSnapshot{
		Image:          meta.Image,
		ImageTag:       meta.ImageTag,
		ManifestDigest: meta.ManifestDigest,
		GitSHA:         meta.GitSHA,
		DeployedAt:     time.Now().UTC(),
	}
	if err := stateMgr.CommitDeployed(workload.Name, newState); err != nil {
		d.logger.Error("failed to commit deployment state", "phase", logger.PhaseCommit, "error", err)
		return state.DeploymentSnapshot{}, fmt.Errorf("commit deployment state: %w", err)
	}

	d.logger.Info("deployment successful",
		"phase", logger.PhaseCommit,
		"workload", workload.Name,
		"digest", meta.ManifestDigest,
		"git_sha", meta.GitSHA,
	)
	return newState, nil
}

func (d *dockerDeployer) rollback(ctx context.Context, stateMgr state.Manager, workload config.Workload, previousState state.DeploymentSnapshot, failedDigest, reason string) (state.DeploymentSnapshot, error) {
	var rollbackState state.DeploymentSnapshot
	d.logger.Info("initiating rollback",
		"phase", logger.PhaseRollback,
		"workload", workload.Name,
		"reason", reason,
		"failed_digest", failedDigest,
	)

	previousStateMeta := git_provider.DeploymentMetadata{
		Image:          previousState.Image,
		ImageTag:       previousState.ImageTag,
		ManifestDigest: previousState.ManifestDigest,
		GitSHA:         previousState.GitSHA,
		CreatedAt:      previousState.DeployedAt,
	}

	if err := d.runStrategy(ctx, workload, previousStateMeta); err != nil {
		if ctx.Err() != nil {
			return rollbackState, ctx.Err()
		}
		return rollbackState, fmt.Errorf("rollback strategy failed: %w (original reason: %s)", err, reason)
	}
	if err := d.runDeployCommand(ctx, workload, previousStateMeta); err != nil {
		if ctx.Err() != nil {
			return rollbackState, ctx.Err()
		}
		d.logger.Error("rollback deploy command failed", "phase", logger.PhaseRollback, "error", err)
		return rollbackState, fmt.Errorf("rollback deploy command failed: %w (original reason: %s)", err, reason)
	}
	if err := d.waitHealthy(ctx, workload); err != nil {
		if ctx.Err() != nil {
			return rollbackState, ctx.Err()
		}
		d.logger.Error("rollback health check failed", "phase", logger.PhaseRollback, "error", err)
		return rollbackState, fmt.Errorf("rollback health check failed: %w (original reason: %s)", err, reason)
	}

	rollbackState = state.DeploymentSnapshot{
		Image:          previousState.Image,
		ImageTag:       previousState.ImageTag,
		ManifestDigest: previousState.ManifestDigest,
		GitSHA:         previousState.GitSHA,
		DeployedAt:     time.Now().UTC(),
		RollbackFrom:   failedDigest,
	}
	if err := stateMgr.CommitRollback(workload.Name, rollbackState); err != nil {
		d.logger.Error("failed to commit rollback state", "phase", logger.PhaseRollback, "error", err)
		return state.DeploymentSnapshot{}, fmt.Errorf("commit rollback state: %w", err)
	}
	return rollbackState, nil
}

func (d *dockerDeployer) waitHealthy(ctx context.Context, workload config.Workload) error {
	check := workload.HealthCheck
	switch check.Type {
	case "", config.HealthCheckNone:
		return nil
	case config.HealthCheckHTTP, config.HealthCheckTCP:
		d.logger.Info("waiting for health",
			"phase", logger.PhaseHealthCheck,
			"workload", workload.Name,
			"type", check.Type,
			"url", check.URL,
			"retries", check.Retries,
		)
		return health.WaitHealthy(ctx, string(check.Type), check.URL, check.Retries, check.Interval.Duration, check.Timeout.Duration)
	case config.HealthCheckCommand:
		return fmt.Errorf("health check type %q is not implemented", check.Type)
	default:
		return fmt.Errorf("unsupported health check type %q", check.Type)
	}
}

func (d *dockerDeployer) runStrategy(ctx context.Context, workload config.Workload, meta git_provider.DeploymentMetadata) error {
	if d.strategy == nil {
		return nil
	}
	return d.strategy.ApplyDeploymentMetadata(ctx, workload, meta)
}

func (d *dockerDeployer) runDeployCommand(ctx context.Context, workload config.Workload, meta git_provider.DeploymentMetadata) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", workload.DeployCommand)
	cmd.Env = append(os.Environ(), deploymentEnv(workload, meta)...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		d.logger.Error("deploy command failed",
			"phase", logger.PhaseRestart,
			"workload", workload.Name,
			"duration_ms", duration.Milliseconds(),
			"stdout", strings.TrimSpace(stdout.String()),
			"stderr", strings.TrimSpace(stderr.String()),
			"error", err,
		)
		return fmt.Errorf("deploy command failed: %w\nstderr: %s", err, strings.TrimSpace(stderr.String()))
	}

	d.logger.Info("deploy command succeeded",
		"phase", logger.PhaseRestart,
		"workload", workload.Name,
		"duration_ms", duration.Milliseconds(),
	)
	return nil
}

func (d *dockerDeployer) containerManifestDigest(ctx context.Context, container dockerContainerInspect) (string, error) {
	if _, digest, ok := strings.Cut(container.Config.Image, "@"); ok && digest != "" {
		return digest, nil
	}

	if container.Image == "" {
		return "", nil
	}

	imageInspect, _, err := d.runDockerOutput(ctx, "runtime_image_inspect", "image", "inspect", container.Image)
	if err != nil {
		return "", fmt.Errorf("inspect runtime image %q: %w", container.Image, err)
	}

	var inspected []dockerImageInspect
	if err := json.Unmarshal(imageInspect, &inspected); err != nil {
		return "", fmt.Errorf("parse image inspect output: %w", err)
	}
	if len(inspected) == 0 {
		return "", nil
	}

	for _, repoDigest := range inspected[0].RepoDigests {
		_, digest, ok := strings.Cut(repoDigest, "@")
		if ok && digest != "" {
			return digest, nil
		}
	}
	return "", nil
}

func isNoSuchContainer(stderr string) bool {
	stderr = strings.ToLower(stderr)
	return strings.Contains(stderr, "no such container") || strings.Contains(stderr, "no such object")
}

type dockerContainerInspect struct {
	Image  string `json:"Image"`
	Config struct {
		Image string `json:"Image"`
	} `json:"Config"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
}

type dockerImageInspect struct {
	RepoDigests []string `json:"RepoDigests"`
}

func deploymentEnv(workload config.Workload, meta git_provider.DeploymentMetadata) []string {
	return []string{
		"WORKLOAD_NAME=" + workload.Name,
		"WORKLOAD_CONTAINER_NAME=" + workload.ContainerName,
		"WORKLOAD_IMAGE=" + meta.Image,
		"WORKLOAD_IMAGE_TAG=" + meta.ImageTag,
		"WORKLOAD_MANIFEST_DIGEST=" + meta.ManifestDigest,
		"WORKLOAD_GIT_SHA=" + meta.GitSHA,
	}
}

func errorShouldTriggerRollback(err error) bool {
	for _, e := range errorsThatCanTriggerRollback {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

func contextIsDone(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
