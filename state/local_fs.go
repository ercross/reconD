package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type deploymentsStateFile struct {
	Deployments []DeploymentStateNew `json:"deployments"`
}

// managerWithLocalFileSystem handles reading and writing deployment state for all workloads.
type managerWithLocalFileSystem struct {
	path string
	mu   sync.Mutex
}

// NewManagerWithLocalFileSystem creates a state manager backed by path.
// It creates the parent directory if it does not exist.
func NewManagerWithLocalFileSystem(path string) (Manager, error) {
	if path == "" {
		return nil, fmt.Errorf("state file path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create state dir for %q: %w", path, err)
	}
	return &managerWithLocalFileSystem{path: path}, nil
}

// LoadDeployed returns the currently deployed state for deploymentName.
func (m *managerWithLocalFileSystem) LoadDeployed(deploymentName string) (DeploymentState, error) {
	if deploymentName == "" {
		return DeploymentState{}, fmt.Errorf("deployment name is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	record, err := m.loadDeployment(deploymentName)
	if err != nil {
		return DeploymentState{}, err
	}
	if isZeroDeploymentState(record.Current) {
		return DeploymentState{}, ErrDeploymentStateNotFound
	}
	return record.Current, nil
}

// LoadPrevious returns the previous deployment state for deploymentName.
func (m *managerWithLocalFileSystem) LoadPrevious(deploymentName string) (DeploymentState, error) {
	if deploymentName == "" {
		return DeploymentState{}, fmt.Errorf("deployment name is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	record, err := m.loadDeployment(deploymentName)
	if err != nil {
		return DeploymentState{}, err
	}
	if isZeroDeploymentState(record.Previous) {
		return DeploymentState{}, ErrDeploymentStateNotFound
	}
	return record.Previous, nil
}

// CommitDeployed atomically persists s as the current deployed state for deploymentName.
// The current state, if any, is promoted to previous first.
func (m *managerWithLocalFileSystem) CommitDeployed(deploymentName string, s DeploymentState) error {
	if deploymentName == "" {
		return fmt.Errorf("deployment name is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	fileState, err := m.loadAll()
	if err != nil && !errors.Is(err, ErrFileNotFound) {
		return err
	}

	record := upsertDeployment(&fileState, deploymentName)
	if !isZeroDeploymentState(record.Current) {
		record.Previous = record.Current
	}
	s.WorkloadName = deploymentName
	record.Current = s

	return m.writeAtomic(fileState)
}

// CommitRollback persists s as the current deployed state after a rollback.
// It does not update Previous, preserving the last known-good rollback target.
func (m *managerWithLocalFileSystem) CommitRollback(deploymentName string, s DeploymentState) error {
	if deploymentName == "" {
		return fmt.Errorf("deployment name is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	fileState, err := m.loadAll()
	if err != nil && !errors.Is(err, ErrFileNotFound) {
		return err
	}

	record := upsertDeployment(&fileState, deploymentName)
	s.WorkloadName = deploymentName
	record.Current = s

	return m.writeAtomic(fileState)
}

func (m *managerWithLocalFileSystem) loadDeployment(deploymentName string) (DeploymentStateNew, error) {
	fileState, err := m.loadAll()
	if err != nil {
		return DeploymentStateNew{}, err
	}

	for _, deployment := range fileState.Deployments {
		if deployment.WorkloadName == deploymentName {
			return deployment, nil
		}
	}
	return DeploymentStateNew{}, ErrDeploymentStateNotFound
}

func (m *managerWithLocalFileSystem) loadAll() (deploymentsStateFile, error) {
	var fileState deploymentsStateFile

	data, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		return fileState, ErrFileNotFound
	}
	if err != nil {
		return fileState, fmt.Errorf("read state %q: %w", m.path, err)
	}
	if len(data) == 0 {
		return fileState, nil
	}

	if err = json.Unmarshal(data, &fileState); err != nil {
		return fileState, fmt.Errorf("parse state %q: %w", m.path, err)
	}
	return fileState, nil
}

func (m *managerWithLocalFileSystem) writeAtomic(fileState deploymentsStateFile) error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o750); err != nil {
		return fmt.Errorf("create state dir for %q: %w", m.path, err)
	}

	data, err := json.MarshalIndent(fileState, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o640); err != nil {
		return fmt.Errorf("write temp state %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, m.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename state %q to %q: %w", tmp, m.path, err)
	}
	return nil
}

func upsertDeployment(fileState *deploymentsStateFile, deploymentName string) *DeploymentStateNew {
	for i := range fileState.Deployments {
		if fileState.Deployments[i].WorkloadName == deploymentName {
			return &fileState.Deployments[i]
		}
	}

	fileState.Deployments = append(fileState.Deployments, DeploymentStateNew{
		WorkloadName: deploymentName,
	})
	return &fileState.Deployments[len(fileState.Deployments)-1]
}

func isZeroDeploymentState(s DeploymentState) bool {
	return s == DeploymentState{}
}
