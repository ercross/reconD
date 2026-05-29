package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// managerWithLocalFileSystem handles reading and writing deployment state for one environment.
type managerWithLocalFileSystem struct {
	dir string
}

// NewManagerWithLocalFileSystem creates a state manager rooted at dir.
// It creates the directory if it does not exist.
func NewManagerWithLocalFileSystem(dir string) (Manager, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create state dir %q: %w", dir, err)
	}
	return &managerWithLocalFileSystem{dir: dir}, nil
}

// LoadDeployed returns the currently deployed state.
// Returns nil, nil if no state has been committed yet (first deployment).
func (m *managerWithLocalFileSystem) LoadDeployed() (DeploymentState, error) {
	return m.load(m.deployedPath())
}

// LoadPrevious returns the previous deployment state.
// Returns nil, nil if there is no previous state (e.g. first rollback attempt).
func (m *managerWithLocalFileSystem) LoadPrevious() (DeploymentState, error) {
	return m.load(m.previousPath())
}

// CommitDeployed atomically persists s as the current deployed state.
// The previous deployed.json (if any) is promoted to previous.json first,
// so we always have a valid rollback target after a successful deployment.
func (m *managerWithLocalFileSystem) CommitDeployed(s DeploymentState) error {
	// Promote current → previous before writing new current.
	if err := m.promoteToPrevious(); err != nil {
		if !errors.Is(err, ErrFileNotFound) {
			return fmt.Errorf("promote previous state: %w", err)
		}
		// No current state to promote — this is the first deployment.
	}
	return m.writeAtomic(m.deployedPath(), s)
}

// CommitRollback persists s as the deployed state after a rollback.
// It does NOT update previous.json, preserving the failed-deployment record
// for post-incident analysis.
func (m *managerWithLocalFileSystem) CommitRollback(s DeploymentState) error {
	return m.writeAtomic(m.deployedPath(), s)
}

// promoteToPrevious copies deployed.json → previous.json.
// This is called before committing a new deployment so we retain the last
// known-good state as a rollback target.
func (m *managerWithLocalFileSystem) promoteToPrevious() error {
	current, err := m.LoadDeployed()
	if err != nil {
		return fmt.Errorf("failed to load current deployed: %w", err)
	}
	return m.writeAtomic(m.previousPath(), current)
}

func (m *managerWithLocalFileSystem) load(path string) (s DeploymentState, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, ErrFileNotFound
	}
	if err != nil {
		return s, fmt.Errorf("read state %q: %w", path, err)
	}

	if err = json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("parse state %q: %w", path, err)
	}
	return s, nil
}

// writeAtomic writes s to path using a write-then-rename pattern.
// This ensures that readers never see a partial write, even if the agent
// crashes mid-write.
func (m *managerWithLocalFileSystem) writeAtomic(path string, s DeploymentState) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	// Write to a temp file in the same directory so the rename is atomic
	// (both source and destination are on the same filesystem).
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o640); err != nil {
		return fmt.Errorf("write temp state %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp) // best-effort cleanup
		return fmt.Errorf("rename state %q → %q: %w", tmp, path, err)
	}
	return nil
}

func (m *managerWithLocalFileSystem) deployedPath() string {
	return filepath.Join(m.dir, "deployed.json")
}
func (m *managerWithLocalFileSystem) previousPath() string {
	return filepath.Join(m.dir, "previous.json")
}
