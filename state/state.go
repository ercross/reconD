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
	LoadDeployed(deploymentName string) (DeploymentSnapshot, error)
	LoadPrevious(deploymentName string) (DeploymentSnapshot, error)
	LoadTerminalFailure(deploymentName string) (TerminalFailureState, error)
	CommitDeployed(deploymentName string, s DeploymentSnapshot) error
	CommitRollback(deploymentName string, s DeploymentSnapshot) error
	CommitTerminalFailure(deploymentName string, s TerminalFailureState) error
	ClearTerminalFailure(deploymentName string) error
}

var (
	ErrFileNotFound            = errors.New("state file not found")
	ErrDeploymentStateNotFound = errors.New("deployment state not found")
)

func IsNotFound(err error) bool {
	return errors.Is(err, ErrFileNotFound) || errors.Is(err, ErrDeploymentStateNotFound)
}

// DeploymentState represents the current and previous deployment for one workload.
type DeploymentState struct {
	WorkloadName    string                `json:"workload_name" yaml:"workload_name"`
	Current         DeploymentSnapshot    `json:"current" yaml:"current"`
	Previous        DeploymentSnapshot    `json:"previous" yaml:"previous"`
	TerminalFailure *TerminalFailureState `json:"terminal_failure,omitempty" yaml:"terminal_failure,omitempty"`
}

// DeploymentSnapshot represents a point-in-time snapshot of what is deployed.
type DeploymentSnapshot struct {
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

// TerminalFailureState blocks automatic retries for a desired deployment that
// exhausted its forward deployment retry budget.
type TerminalFailureState struct {
	ManifestDigest            string    `json:"manifest_digest"`
	GitSHA                    string    `json:"git_sha,omitempty"`
	ForwardDeploymentAttempts int       `json:"forward_deployment_attempts"`
	RollbackAttempts          int       `json:"rollback_attempts,omitempty"`
	Reason                    string    `json:"reason"`
	FailedAt                  time.Time `json:"failed_at"`
}
