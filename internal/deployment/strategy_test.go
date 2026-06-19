package deployment

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/ercross/reconD/internal/config"
	"github.com/ercross/reconD/internal/git_provider"
	"github.com/stretchr/testify/require"
)

func TestEnvFileStrategyUpdatesImageTag(t *testing.T) {
	t.Parallel()

	envPath := filepath.Join(t.TempDir(), ".env")
	require.NoError(t, os.WriteFile(envPath, []byte("OTHER=value\nIMAGE_TAG=old\n"), 0o600))

	strategy := NewStrategy(config.Strategy{
		Type:        config.StrategyEnvFile,
		EnvFilePath: envPath,
		ImageTagKey: "IMAGE_TAG",
	}, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	err := strategy.ApplyDeploymentMetadata(context.Background(), config.Workload{Name: "api"}, git_provider.DeploymentMetadata{
		ImageTag: "prod-sha-10a3e42",
	})
	require.NoError(t, err)

	updated, err := os.ReadFile(envPath)
	require.NoError(t, err)
	require.Equal(t, "OTHER=value\nIMAGE_TAG=prod-sha-10a3e42\n", string(updated))
}

func TestEnvFileStrategyAppendsDefaultImageTagKey(t *testing.T) {
	t.Parallel()

	envPath := filepath.Join(t.TempDir(), ".env")
	strategy := NewStrategy(config.Strategy{
		Type:        config.StrategyEnvFile,
		EnvFilePath: envPath,
		ImageTagKey: "IMAGE_TAG",
	}, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	err := strategy.ApplyDeploymentMetadata(context.Background(), config.Workload{Name: "api"}, git_provider.DeploymentMetadata{
		ImageTag: "prod-sha-10a3e42",
	})
	require.NoError(t, err)

	updated, err := os.ReadFile(envPath)
	require.NoError(t, err)
	require.Equal(t, "IMAGE_TAG=prod-sha-10a3e42\n", string(updated))
}
