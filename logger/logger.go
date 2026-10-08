// Package logger configures structured logging for the agent using log/slog.
//
// Design decisions:
//   - JSON output by default for log aggregation (journald, Loki, Datadog)
//   - Text output when log.format=text (local development)
//   - Level controlled by log.level (debug, info, warn, error)
//   - Structured fields: workload, phase, digest, duration included consistently
package logger

import (
	"log/slog"
	"os"
	"strings"

	"github.com/ercross/reconD/config"
)

// Setup configures the default global slog logger.
// Call once at startup before spawning goroutines.
func Setup(cfg config.Log) *slog.Logger {
	level := parseLevel(cfg.Level)
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if strings.ToLower(cfg.Format) == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		// JSON is the default: structured logs integrate with journald and
		// any log shipping agent (Promtail, Fluentd, Datadog agent, etc).
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// WithContainerName returns a child logger with the container field pre-set.
// All log lines emitted from the reconciler for a container will carry this.
func WithContainerName(logger *slog.Logger, containerName string) *slog.Logger {
	return logger.With("container", containerName)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Phase constants for structured log fields.
// Using constants prevents typos and enables log-based alerting rules.
const (
	PhasePoll        = "poll"
	PhaseDrift       = "drift_check"
	PhasePull        = "image_pull"
	PhaseStrategy    = "deployment_strategy"
	PhaseRestart     = "restart"
	PhaseHealthCheck = "health_check"
	PhaseCommit      = "commit"
	PhaseRollback    = "rollback"
	PhaseNoop        = "noop"
)
