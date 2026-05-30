package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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

	previousState, err := stateMgr.LoadDeployed()
	if err != nil && !errors.Is(err, state.ErrFileNotFound) {
		d.logger.Warn("could not load previous state", "error", err)
	}
	noPreviousState := errors.Is(err, state.ErrFileNotFound)

	d.notifier.NotifyOnNewDeploymentStarted(meta)

	newState, err := d.deploy(ctx, workload, meta, stateMgr)
	if err == nil {
		d.notifier.NotifyOnDeploymentSuccess(newState)
		if err = d.PruneDanglingImages(ctx, workload.Labels); err != nil {
			d.logger.Error("failed to prune dangling images", "phase", logger.PhaseCommit, "workload", workload.Name, "error", err)
		}
		return nil
	}

	d.notifier.NotifyOnDeploymentFailed(meta, err)
	d.logger.Error("deployment failed", "workload", workload.Name, "error", err)
	if !errorShouldTriggerRollback(err) {
		return fmt.Errorf("deployment failed but no rollback required: %w", err)
	}

	if noPreviousState {
		d.logger.Error("no previous state available for rollback", "phase", logger.PhaseRollback)
		return fmt.Errorf("rollback failed: no previous deployment state")
	}

	rollbackState, rollbackErr := d.rollback(ctx, stateMgr, workload, previousState, meta.ManifestDigest, err.Error())
	if rollbackErr == nil {
		d.notifier.NotifyOnDeploymentSuccess(rollbackState)
		return nil
	}

	d.notifier.NotifyOnDeploymentFailed(meta, fmt.Errorf("ROLLBACK FAILED: %w", rollbackErr))
	return fmt.Errorf("deployment and rollback failed: %w", rollbackErr)
}

func (d *dockerDeployer) deploy(ctx context.Context, workload config.Workload, meta git_provider.DeploymentMetadata, stateMgr state.Manager) (state.DeploymentState, error) {
	var newState state.DeploymentState

	if err := d.runStrategy(ctx, workload, meta); err != nil {
		return newState, errors.Join(errDeploymentStrategyFailed, err)
	}

	d.logger.Info("running deploy command", "phase", logger.PhaseRestart, "workload", workload.Name)
	if err := d.runDeployCommand(ctx, workload, meta); err != nil {
		return newState, errors.Join(errDeployCommandFailed, err)
	}

	if err := d.waitHealthy(ctx, workload); err != nil {
		return newState, errors.Join(errHealthCheckFailed, err)
	}

	newState = state.DeploymentState{
		Workload:       workload.Name,
		Environment:    meta.Environment,
		Image:          meta.Image,
		ImageTag:       meta.ImageTag,
		ManifestDigest: meta.ManifestDigest,
		GitSHA:         meta.GitSHA,
		DeployedAt:     time.Now().UTC(),
	}
	if err := stateMgr.CommitDeployed(newState); err != nil {
		d.logger.Error("failed to commit deployment state", "phase", logger.PhaseCommit, "error", err)
	}

	d.logger.Info("deployment successful",
		"phase", logger.PhaseCommit,
		"workload", workload.Name,
		"digest", meta.ManifestDigest,
		"git_sha", meta.GitSHA,
	)
	return newState, nil
}

func (d *dockerDeployer) rollback(ctx context.Context, stateMgr state.Manager, workload config.Workload, previousState state.DeploymentState, failedDigest, reason string) (state.DeploymentState, error) {
	var rollbackState state.DeploymentState
	d.logger.Info("initiating rollback",
		"phase", logger.PhaseRollback,
		"workload", workload.Name,
		"reason", reason,
		"failed_digest", failedDigest,
	)

	previousStateMeta := git_provider.DeploymentMetadata{
		Environment:    previousState.Environment,
		Image:          previousState.Image,
		ImageTag:       previousState.ImageTag,
		ManifestDigest: previousState.ManifestDigest,
		GitSHA:         previousState.GitSHA,
		CreatedAt:      previousState.DeployedAt,
	}

	if err := d.runStrategy(ctx, workload, previousStateMeta); err != nil {
		return rollbackState, fmt.Errorf("rollback strategy failed: %w (original reason: %s)", err, reason)
	}
	if err := d.runDeployCommand(ctx, workload, previousStateMeta); err != nil {
		d.logger.Error("rollback deploy command failed", "phase", logger.PhaseRollback, "error", err)
		return rollbackState, fmt.Errorf("rollback deploy command failed: %w (original reason: %s)", err, reason)
	}
	if err := d.waitHealthy(ctx, workload); err != nil {
		d.logger.Error("rollback health check failed", "phase", logger.PhaseRollback, "error", err)
		return rollbackState, fmt.Errorf("rollback health check failed: %w (original reason: %s)", err, reason)
	}

	rollbackState = state.DeploymentState{
		Workload:       workload.Name,
		Environment:    previousState.Environment,
		Image:          previousState.Image,
		ImageTag:       previousState.ImageTag,
		ManifestDigest: previousState.ManifestDigest,
		GitSHA:         previousState.GitSHA,
		DeployedAt:     time.Now().UTC(),
		RollbackFrom:   failedDigest,
	}
	if err := stateMgr.CommitRollback(rollbackState); err != nil {
		d.logger.Error("failed to commit rollback state", "phase", logger.PhaseRollback, "error", err)
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
		"WORKLOAD_ENVIRONMENT=" + meta.Environment,
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
