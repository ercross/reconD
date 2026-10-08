// Package state manages persistent deployment state for all workloads.
// State is stored as a JSON file on disk, making it inspectable by operators
// and safe across agent restarts.
//
// File layout:
//
//	{state_file} — the current and previous deployment for every workload
//
// The write path uses atomic rename to prevent partial writes from corrupting
// the state file during a crash.
package state

import (
	"errors"
	"time"
)

type Manager interface {
	LoadDeployed(deploymentName string) (DeploymentState, error)
	LoadPrevious(deploymentName string) (DeploymentState, error)
	CommitDeployed(deploymentName string, s DeploymentState) error
	CommitRollback(deploymentName string, s DeploymentState) error
}

var (
	ErrFileNotFound            = errors.New("state file not found")
	ErrDeploymentStateNotFound = errors.New("deployment state not found")
)

func IsNotFound(err error) bool {
	return errors.Is(err, ErrFileNotFound) || errors.Is(err, ErrDeploymentStateNotFound)
}

// DeploymentStateNew represents the current and previous deployment for one workload.
type DeploymentStateNew struct {
	WorkloadName string          `json:"workload_name" yaml:"workload_name"`
	Current      DeploymentState `json:"current" yaml:"current"`
	Previous     DeploymentState `json:"previous" yaml:"previous"`
}

// DeploymentState represents a point-in-time snapshot of what is deployed.
type DeploymentState struct {
	WorkloadName string `json:"workload_name"`

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
