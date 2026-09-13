package git_provider

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNoDeploymentMetaFound = errors.New("no deployment metadata found")
)

// DeploymentMetadata is the structure published as a release asset by CI.
// The GitHub Actions workflow serializes this after a successful image push.
type DeploymentMetadata struct {
	// Environment is the target environment name (e.g. "dev", "production").
	Environment string `json:"environment"`

	// Image is the full image reference including tag.
	Image string `json:"image"`

	// ImageTag is the Docker image tag (e.g. "dev-latest", "prod-latest").
	ImageTag string `json:"image_tag"`

	// ManifestDigest is the immutable OCI digest of the pushed image.
	// sha256:... — this is the canonical key for drift detection.
	ManifestDigest string `json:"manifest_digest"`

	// GitSHA is the commit SHA that triggered this release.
	GitSHA string `json:"git_sha"`

	// CreatedAt is when CI published this metadata.
	CreatedAt time.Time `json:"created_at"`
}

type GitProvider interface {
	FetchLatestDeploymentMetadata(ctx context.Context, environment string) (meta DeploymentMetadata, err error)
}
