// Package notifier sends deployment lifecycle notifications to external
// systems.
package notifier

import (
	"github.com/ercross/reconD/internal/git_provider"
	"github.com/ercross/reconD/internal/state"
)

// Notifier sends deployment state notifications or alert to external destinations like Slack.
//
// Since alerts are not core business logic, error encountered should be logged
type Notifier interface {
	NotifyOnNewDeploymentStarted(meta git_provider.DeploymentMetadata)
	NotifyOnDeploymentFailed(meta git_provider.DeploymentMetadata, err error)
	NotifyOnDeploymentSuccess(dep state.DeploymentState)
}

type Noop struct{}

func (Noop) NotifyOnNewDeploymentStarted(meta git_provider.DeploymentMetadata)        {}
func (Noop) NotifyOnDeploymentFailed(meta git_provider.DeploymentMetadata, err error) {}
func (Noop) NotifyOnDeploymentSuccess(dep state.DeploymentState)                      {}
