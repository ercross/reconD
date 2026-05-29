package deployment_orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ercross/reconD/config"
	"github.com/ercross/reconD/git_provider"
	"github.com/ercross/reconD/health"
	"github.com/ercross/reconD/logger"
	"github.com/ercross/reconD/notifier"
	"github.com/ercross/reconD/state"
)

type dockerOrchestrator struct {
	service  config.Service
	logger   *slog.Logger
	notifier notifier.Notifier
}

func NewDockerOrchestrator(
	service config.Service,
	notifier notifier.Notifier,
	log *slog.Logger,
) DeploymentOrchestrator {
	return &dockerOrchestrator{
		service:  service,
		logger:   log,
		notifier: notifier,
	}
}

func (d *dockerOrchestrator) Deploy(ctx context.Context, target config.Service, meta git_provider.DeploymentMetadata, stateMgr state.Manager) error {
	d.logger.Info("starting deployment",
		"phase", logger.PhasePull,
		"service", target.Name,
		"digest", meta.ManifestDigest,
		"git_sha", meta.GitSHA,
	)

	previousState, err := stateMgr.LoadDeployed()
	if err != nil && !errors.Is(err, state.ErrFileNotFound) {
		d.logger.Warn("could not load previous state", "error", err)
	}
	noPreviousState := errors.Is(err, state.ErrFileNotFound)

	d.notifier.NotifyOnNewDeploymentStarted(meta)

	newState, err := d.deploy(ctx, target, meta, stateMgr)
	if err == nil {
		d.notifier.NotifyOnDeploymentSuccess(newState)
		if err = d.PruneDanglingImages(ctx, target.Labels); err != nil {
			d.logger.Error("failed to prune dangling images", "phase", logger.PhaseCommit, "service", target.Name, "error", err)
		}
		return nil
	}

	d.notifier.NotifyOnDeploymentFailed(meta, err)
	d.logger.Error("deployment failed", "service", target.Name, "error", err)
	if !errorShouldTriggerRollback(err) {
		return fmt.Errorf("deployment failed but no rollback required: %w", err)
	}

	if noPreviousState {
		d.logger.Error("no previous state available for rollback", "phase", logger.PhaseRollback)
		return fmt.Errorf("rollback failed: no previous deployment state")
	}

	rollbackState, rollbackErr := d.rollback(ctx, stateMgr, target, previousState, meta.ManifestDigest, err.Error())
	if rollbackErr == nil {
		d.notifier.NotifyOnDeploymentSuccess(rollbackState)
		return nil
	}

	d.notifier.NotifyOnDeploymentFailed(meta, fmt.Errorf("ROLLBACK FAILED: %w", rollbackErr))
	return fmt.Errorf("deployment and rollback failed: %w", rollbackErr)
}

func (d *dockerOrchestrator) deploy(ctx context.Context, target config.Service, meta git_provider.DeploymentMetadata, stateMgr state.Manager) (state.DeploymentState, error) {
	var newState state.DeploymentState

	if err := d.updateComposeImageTags(target, meta); err != nil {
		return newState, errors.Join(errComposeEnvFailed, err)
	}

	d.logger.Info("pulling image", "phase", logger.PhasePull, "service", target.Name, "image", meta.Image)
	if err := d.PullImage(ctx, target.Name); err != nil {
		return newState, errors.Join(errImagePullFailed, err)
	}

	if err := d.ensureDependenciesRunning(ctx); err != nil {
		return newState, errors.Join(errRestartFailed, err)
	}
	if err := d.runBeforeMainServices(ctx); err != nil {
		return newState, errors.Join(errMigrationFailed, err)
	}

	d.logger.Info("starting target service", "phase", logger.PhaseRestart, "service", target.Name)
	if err := d.UpServices(ctx, logger.PhaseRestart, []string{target.Name}, true, PullPolicyNever); err != nil {
		return newState, errors.Join(errRestartFailed, err)
	}

	reconciler := target.Reconciler
	if reconciler == nil {
		return newState, fmt.Errorf("target service %q has no reconciler", target.Name)
	}
	d.logger.Info("waiting for health",
		"phase", logger.PhaseHealthCheck,
		"service", target.Name,
		"url", reconciler.HealthCheckURL,
		"retries", reconciler.HealthCheck.MaxRetries,
	)
	if err := health.WaitHealthy(
		ctx,
		reconciler.HealthCheckURL,
		reconciler.HealthCheck.MaxRetries,
		reconciler.HealthCheck.Interval,
		reconciler.HealthCheck.Timeout,
	); err != nil {
		return newState, errors.Join(errHealthCheckFailed, err)
	}

	newState = state.DeploymentState{
		Environment:    d.env.Name,
		ServiceName:    target.Name,
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
		"service", target.Name,
		"digest", meta.ManifestDigest,
		"git_sha", meta.GitSHA,
	)
	return newState, nil
}

func (d *dockerOrchestrator) ensureDependenciesRunning(ctx context.Context) error {
	dependencies := d.servicesByRole(config.ServiceRoleDependency)
	if len(dependencies) == 0 {
		return nil
	}
	d.logger.Info("ensuring dependency groups", "phase", logger.PhaseRestart)
	if err := d.UpServices(ctx, "dependency_start", servicesNames(dependencies), false, PullPolicyMissing); err != nil {
		return err
	}
	runtimeState, err := d.CurrentRuntimeState(ctx, containerNames(dependencies))
	if err != nil {
		return err
	}
	if !runtimeState.Running {
		return fmt.Errorf("dependency groups are not running")
	}
	return nil
}

func containerNames(services []config.Service) []string {
	var names []string
	for _, service := range services {
		names = append(names, service.ContainerName)
	}
	return names
}

func (d *dockerOrchestrator) runBeforeMainServices(ctx context.Context) error {
	mustRunBeforeMainServices := d.servicesByRole(config.ServiceRoleRunBeforeMain)
	for _, service := range mustRunBeforeMainServices {
		d.logger.Info("running pre-main service", "phase", logger.PhaseMigration, "role", config.ServiceRoleRunBeforeMain, "service", service.Name)
		if err := d.PullImage(ctx, service.Name); err != nil {
			return errors.Join(errImagePullFailed, err)
		}
		timeout := time.Duration(service.MaxRuntimeSeconds) * time.Second
		if err := d.RunMigrations(ctx, service.Name, timeout); err != nil {
			return err
		}
	}

	return nil
}

func (d *dockerOrchestrator) updateComposeImageTags(target config.Service, meta git_provider.DeploymentMetadata) error {
	if d.env.ComposeEnvFilePath == "" {
		return fmt.Errorf("compose env file path is required")
	}
	if meta.ImageTag == "" {
		return fmt.Errorf("deployment metadata image_tag is required")
	}
	key := target.ImageTagEnvKey
	if key == "" {
		key = "IMAGE_TAG"
	}
	d.logger.Info("updating compose image tag", "phase", logger.PhasePull, "service", target.Name, "env_key", key, "image_tag", meta.ImageTag)
	if err := upsertEnvFileValue(d.env.ComposeEnvFilePath, key, meta.ImageTag); err != nil {
		return err
	}
	return nil
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write temp state %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename state %q to %q: %w", tmp, path, err)
	}
	return nil
}

func upsertEnvFileValue(path, key, value string) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read env file %q: %w", path, err)
	}

	content := string(data)
	lines := []string{}
	if content != "" {
		lines = strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	}
	replacement := key + "=" + value
	found := false

	for i, line := range lines {
		if envLineKey(line) != key {
			continue
		}
		lines[i] = replacement
		found = true
		break
	}

	if !found {
		if content == "" {
			lines = []string{replacement}
		} else {
			lines = append(lines, replacement)
		}
	}

	updated := strings.Join(lines, "\n")
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create env file dir %q: %w", filepath.Dir(path), err)
	}
	if err := writeAtomic(path, []byte(updated)); err != nil {
		return fmt.Errorf("write env file %q: %w", path, err)
	}
	return nil
}

func envLineKey(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return ""
	}
	trimmed = strings.TrimPrefix(trimmed, "export ")
	before, _, ok := strings.Cut(trimmed, "=")
	if !ok {
		return ""
	}
	return strings.TrimSpace(before)
}

func (d *dockerOrchestrator) rollback(ctx context.Context, stateMgr state.Manager, target config.Service, previousState state.DeploymentState, failedDigest, reason string) (state.DeploymentState, error) {
	var rollbackState state.DeploymentState
	d.logger.Info("initiating rollback",
		"phase", logger.PhaseRollback,
		"service", target.Name,
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
	if err := d.updateComposeImageTags(target, previousStateMeta); err != nil {
		d.logger.Warn("could not update compose image tag", "error", err)
	}

	if err := d.UpServices(ctx, logger.PhaseRollback, []string{target.Name}, true, PullPolicyNever); err != nil {
		d.logger.Error("rollback restart failed", "phase", logger.PhaseRollback, "error", err)
		return rollbackState, fmt.Errorf("rollback restart failed: %w (original reason: %s)", err, reason)
	}

	reconciler := target.Reconciler
	if reconciler == nil {
		return rollbackState, fmt.Errorf("rollback failed: target service %q has no reconciler", target.Name)
	}
	if err := health.WaitHealthy(
		ctx,
		reconciler.HealthCheckURL,
		reconciler.HealthCheck.MaxRetries,
		reconciler.HealthCheck.Interval,
		reconciler.HealthCheck.Timeout,
	); err != nil {
		d.logger.Error("rollback health check failed", "phase", logger.PhaseRollback, "error", err)
		return rollbackState, fmt.Errorf("rollback health check failed: %w (original reason: %s)", err, reason)
	}

	rollbackState = state.DeploymentState{
		Environment:    d.env.Name,
		ServiceName:    target.Name,
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

func (d *dockerOrchestrator) servicesByRole(role config.ServiceRole) []config.Service {
	var services []config.Service
	for _, service := range d.services {
		if service.Role == role {
			services = append(services, service)
		}
	}
	return services
}

func servicesNames(services []config.Service) []string {
	var names []string
	for _, service := range services {
		names = append(names, service.Name)
	}
	return names
}

func errorShouldTriggerRollback(err error) bool {
	for _, e := range errorsThatCanTriggerRollback {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
