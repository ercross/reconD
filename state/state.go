// Package state manages persistent deployment state for each workload.
// State is stored as JSON files on disk, making it inspectable by operators
// and safe across agent restarts.
//
// File layout per workload:
//
//	{state_dir}/deployed.json  — the currently active deployment
//	{state_dir}/previous.json  — the deployment before the last successful one
//
// The write path uses atomic rename to prevent partial writes from corrupting
// the state file during a crash.
package state

import (
	"errors"
	"time"
)

type Manager interface {
	LoadDeployed() (DeploymentState, error)
	LoadPrevious() (DeploymentState, error)
	CommitDeployed(s DeploymentState) error
	CommitRollback(s DeploymentState) error
}

var ErrFileNotFound = errors.New("file not found")

// DeploymentState represents a point-in-time snapshot of what is deployed.
type DeploymentState struct {
	Workload string `json:"workload"`

	// Image is the full image reference that is deployed.
	Image string `json:"image"`

	ImageTag string `json:"image_tag"`

	// ManifestDigest is the OCI manifest digest (sha256:...) of the deployed image.
	// This is the canonical drift-detection key — image tags are mutable,
	// digests are not.
	ManifestDigest string `json:"manifest_digest"`

	// GitSHA is the source commit that produced this image.
	GitSHA string `json:"git_sha"`

	// DeployedAt is when this state was committed (after successful health check).
	DeployedAt time.Time `json:"deployed_at"`

	// RollbackFrom is set when this state was restored by a rollback.
	// It contains the digest that failed, for audit purposes.
	RollbackFrom string `json:"rollback_from,omitempty"`
}
