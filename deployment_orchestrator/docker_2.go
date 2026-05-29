// Package deployment_orchestrator implements the deployment execution layer.
// It translates high-level deployment operations into docker compose commands
// executed via exec.Command.
//
// Architecture note: We intentionally shell out to `docker compose` rather
// than using the Docker SDK. This approach:
//  1. Keeps the agent simple and auditable — any operator can understand what
//     it does by reading the commands it runs.
//  2. Avoids Docker SDK version coupling — compose file compatibility is
//     Docker's problem, not ours.
//  3. Makes it easy to test manually by running the same commands by hand.
//
// Future: Replace exec-based compose calls with Docker SDK if finer-grained
// control (streaming logs, event subscription) becomes necessary.
package deployment_orchestrator

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// PullImage pulls the specified image using docker compose pull.
// This ensures the image is available locally before restarting services.
// Using `compose pull` instead of `docker pull` respects compose service
// configuration (e.g. platform, pull policy).
func (e *dockerOrchestrator) PullImage(ctx context.Context, service string) error {
	err := e.run(ctx, "image_pull", "pull", "--quiet", service)
	if err != nil {
		return errors.Join(errImagePullFailed, err)
	}

	return nil
}

const (
	PullPolicyNever   = "never"
	PullPolicyMissing = "missing"
)

func (e *dockerOrchestrator) PruneDanglingImages(ctx context.Context, targetLabels map[string]string) error {

	args := []string{"image", "prune", "--force", "-a"}

	for key, value := range targetLabels {
		filterExpr := fmt.Sprintf("label=%s=%s", key, value)
		args = append(args, "--filter", filterExpr)
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	e.logger.Debug("pruning dangling images", "phase", "image_prune")

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	if err != nil {
		e.logger.Error("docker image prune failed",
			"phase", "image_prune",
			"duration_ms", duration.Milliseconds(),
			"stderr", strings.TrimSpace(stderr.String()),
			"error", err,
		)
		return fmt.Errorf("docker image prune: %w\nstderr: %s",
			err, strings.TrimSpace(stderr.String()))
	}

	e.logger.Info("dangling images pruned",
		"phase", "image_prune",
		"duration_ms", duration.Milliseconds(),
		"reclaimed", strings.TrimSpace(stdout.String()),
	)
	return nil
}

// run executes a docker compose command with the configured files and env files.
// It captures stdout and stderr for logging and error reporting.
func (e *dockerOrchestrator) run(ctx context.Context, phase string, args ...string) error {

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = os.Environ() // Inherit the agent's environment variables.

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	e.logger.Debug("executing docker compose",
		"phase", phase,
		"args", strings.Join(args, " "),
	)

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	if err != nil {
		e.logger.Error("docker compose command failed",
			"phase", phase,
			"duration_ms", duration.Milliseconds(),
			"stdout", strings.TrimSpace(stdout.String()),
			"stderr", strings.TrimSpace(stderr.String()),
			"error", err,
		)
		return fmt.Errorf("docker compose %s failed: %w\nstderr: %s",
			phase, err, strings.TrimSpace(stderr.String()))
	}

	e.logger.Info("docker compose command succeeded",
		"phase", phase,
		"duration_ms", duration.Milliseconds(),
	)
	return nil
}

func (e *dockerOrchestrator) CurrentRuntimeState(ctx context.Context, containerNames []string) (RuntimeState, error) {
	running, err := e.getServiceState(ctx, containerNames)
	if err != nil {
		return RuntimeState{}, err
	}
	if !running {
		return RuntimeState{Running: false}, nil
	}

	return RuntimeState{
		Running: true,
	}, nil
}

func (e *dockerOrchestrator) getServiceState(ctx context.Context, containerNames []string) (running bool, err error) {
	if len(containerNames) == 0 {
		return false, nil
	}

	cli, err := client.NewClientWithOpts(
		client.WithHost("unix://"+os.ExpandEnv("$HOME/.docker/run/docker.sock")),
		client.WithAPIVersionNegotiation())
	if err != nil {
		return false, fmt.Errorf("create docker client: %w", err)
	}
	defer cli.Close()

	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All: true, // include exited/paused, not just running
		//Filters: f,
	})
	if err != nil {
		return false, fmt.Errorf("list containers: %w", err)
	}

	// Index by service name
	seen := map[string]container.Summary{}
	for _, c := range containers {
		//svcName := c.Labels["com.docker.compose.service"]
		//seen[svcName] = c
		for _, name := range c.Names {
			seen[strings.TrimPrefix(name, "/")] = c
		}
	}

	for _, svc := range containerNames {
		c, ok := seen[svc]
		if !ok {
			return false, nil // service container doesn't exist
		}
		if c.State != "running" {
			return false, nil
		}
		// Health: Status is "healthy", "unhealthy", "starting", or "" (no healthcheck)
		if c.Status != "" && c.State == "running" {
			inspect, err := cli.ContainerInspect(ctx, c.ID)
			if err != nil {
				return false, fmt.Errorf("inspect container %s: %w", c.ID, err)
			}
			if h := inspect.State.Health; h != nil {
				if h.Status != "healthy" {
					return false, nil
				}
			}
		}
	}

	return true, nil
}

// composePSEntry is the JSON shape emitted by `docker compose ps --format json`.
// Only the fields we need are mapped; unknown fields are silently ignored.
type composePSEntry struct {
	// ID is the short container ID.
	ID string `json:"ID"`
	// Service is the compose service name (e.g. "app").
	Service string `json:"Service"`
	// State is the container state string: "running", "exited", "paused", etc.
	State string `json:"State"`
	// Health is "healthy", "unhealthy", or empty when the service has no healthcheck.
	Health string `json:"Health"`
}

// getServiceState runs `docker compose ps --format json` for the expected
// services and reports whether they all exist and are in state "running".
// It returns the first service container ID on success.
//
// `compose ps` output is one JSON object per line (NDJSON), not a JSON array.
// Each line represents one replica of a service.
// getServiceState — handle both JSON array and NDJSON from compose ps
func (e *dockerOrchestrator) getServiceStateOld(ctx context.Context, services []string) (containerID string, running bool, err error) {
	args := []string{
		"ps",
		"--format", "json",
		"--status", "running",
		"--status", "exited",
		"--status", "paused",
		"--status", "dead",
	}
	args = append(args, services...)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", false, fmt.Errorf("docker compose ps: %w\nstderr: %s",
			err, strings.TrimSpace(stderr.String()))
	}

	entries, err := parseComposePSOutput(stdout.Bytes())
	if err != nil {
		return "", false, fmt.Errorf("parse compose ps output: %w", err)
	}

	expected := map[string]struct{}{}
	for _, service := range services {
		expected[service] = struct{}{}
	}
	seen := map[string]composePSEntry{}
	for _, entry := range entries {
		if _, ok := expected[entry.Service]; ok || len(expected) == 0 {
			seen[entry.Service] = entry
		}
	}

	for _, service := range services {
		entry, ok := seen[service]
		if !ok {
			return "", false, nil
		}
		if entry.State != "running" {
			return entry.ID, false, nil
		}
		if entry.Health != "" && entry.Health != "healthy" {
			return entry.ID, false, nil
		}
	}
	if len(services) == 0 {
		return "", false, nil
	}
	return seen[services[0]].ID, true, nil
}

// parseComposePSOutput handles both output shapes docker compose ps emits:
//   - JSON array:  [{"ID":"abc","Service":"app","State":"running"}, ...]
//   - NDJSON:       {"ID":"abc","Service":"app","State":"running"}\n...
//
// Compose v2 switched from array to NDJSON at some point and the version in
// the wild varies by distro/install method, so we must tolerate both.
func parseComposePSOutput(data []byte) ([]composePSEntry, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil
	}

	// A JSON array starts with '['; NDJSON starts with '{'.
	if trimmed[0] == '[' {
		var entries []composePSEntry
		if err := json.Unmarshal(trimmed, &entries); err != nil {
			return nil, fmt.Errorf("parse JSON array: %w", err)
		}
		return entries, nil
	}

	// NDJSON: one JSON object per line.
	var entries []composePSEntry
	scanner := bufio.NewScanner(bytes.NewReader(trimmed))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var entry composePSEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("parse NDJSON line %q: %w", line, err)
		}
		entries = append(entries, entry)
	}
	return entries, scanner.Err()
}

// inspectImageDigest runs `docker inspect` on the container and returns the
// RepoDigest (sha256:...) of the image it was started from.
//
// `docker inspect` returns a JSON array; we always take index [0].
//
// The digest is extracted from Image.RepoDigests rather than Image.ID because:
//   - Image.ID is the config digest (sha256 of the image config blob), which
//     differs from the manifest digest that CI publishes.
//   - RepoDigests contains the registry manifest digest in the form
//     "registry/image@sha256:..." — this matches what GitHub Actions captures
//     via `docker buildx build --iidfile` or the push output.
func (e *dockerOrchestrator) inspectImageDigest(ctx context.Context, containerID string) (string, error) {
	// --format with a Go template avoids parsing the full 200-line inspect blob.
	const tmpl = `{{index (index .RepoDigests 0) | printf "%s"}}`

	cmd := exec.CommandContext(ctx,
		"docker", "inspect",
		"--format", `{{index .Image}}`,
		containerID,
	)
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker inspect %s: %w\nstderr: %s",
			containerID, err, strings.TrimSpace(stderr.String()))
	}

	// `docker inspect --format '{{.Image}}'` returns the image content-
	// addressable ID: sha256:<config-digest>. We then resolve its RepoDigest.
	imageID := strings.TrimSpace(stdout.String())
	if imageID == "" {
		return "", fmt.Errorf("docker inspect %s: empty image ID", containerID)
	}

	return e.resolveRepoDigest(ctx, imageID)
}

// resolveRepoDigest — use a conditional template so Docker never indexes
// into an empty RepoDigests slice
func (e *dockerOrchestrator) resolveRepoDigest(ctx context.Context, imageID string) (string, error) {
	// {{if .RepoDigests}} guards the index call inside Docker's template engine.
	// Without it, {{index .RepoDigests 0}} causes a template execution error
	// when the slice is empty (e.g. image loaded via docker load), and the
	// process exits non-zero before our fallback logic can run.
	const tmpl = `{{if .RepoDigests}}{{index .RepoDigests 0}}{{end}}`

	cmd := exec.CommandContext(ctx,
		"docker", "image", "inspect",
		"--format", tmpl,
		imageID,
	)
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker image inspect %s: %w\nstderr: %s",
			imageID, err, strings.TrimSpace(stderr.String()))
	}

	raw := strings.TrimSpace(stdout.String())
	if raw == "" {
		// Empty output means RepoDigests was empty — image has no registry
		// provenance (loaded from tarball, built locally, etc.).
		// Fall back to the image config ID; it won't match a CI manifest
		// digest so drift will always be detected — the safe default.
		e.logger.Warn("image has no RepoDigest, falling back to image ID",
			"phase", "runtime_observe",
			"image_id", imageID,
		)
		return imageID, nil
	}

	// RepoDigests[0] is "ghcr.io/toughbred/gymportal@sha256:abc123..."
	// Strip the registry/name@ prefix; keep only the sha256:... portion.
	if _, digest, found := strings.Cut(raw, "@"); found {
		return digest, nil
	}

	e.logger.Warn("unexpected RepoDigest format, returning raw value",
		"phase", "runtime_observe",
		"raw", raw,
	)
	return raw, nil
}
