package deployment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ercross/reconD/internal/config"
	"github.com/ercross/reconD/internal/git_provider"
	"github.com/ercross/reconD/internal/logger"
)

type MetadataApplicationStrategy interface {
	ApplyDeploymentMetadata(ctx context.Context, workload config.Workload, meta git_provider.DeploymentMetadata) error
}

func NewStrategy(cfg config.Strategy, log *slog.Logger) MetadataApplicationStrategy {
	switch cfg.Type {
	case "", config.StrategyNone:
		return nil
	case config.StrategyEnvFile:
		return envFileStrategy{cfg: cfg, log: log}
	default:
		return nil
	}
}

type envFileStrategy struct {
	cfg config.Strategy
	log *slog.Logger
}

func (s envFileStrategy) ApplyDeploymentMetadata(ctx context.Context, workload config.Workload, meta git_provider.DeploymentMetadata) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if meta.ImageTag == "" {
		return fmt.Errorf("deployment metadata image_tag is required")
	}

	s.log.Info("updating deployment env file",
		"phase", logger.PhaseStrategy,
		"workload", workload.Name,
		"path", s.cfg.EnvFilePath,
		"key", s.cfg.ImageTagKey,
		"image_tag", meta.ImageTag,
	)

	if err := upsertEnvFileValue(s.cfg.EnvFilePath, s.cfg.ImageTagKey, meta.ImageTag); err != nil {
		return err
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
		lines = append(lines, replacement)
	}

	updated := strings.Join(lines, "\n")
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create env file dir %q: %w", filepath.Dir(path), err)
	}
	return writeAtomic(path, []byte(updated))
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

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write temp env file %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename env file %q to %q: %w", tmp, path, err)
	}
	return nil
}
