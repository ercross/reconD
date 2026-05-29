// Package logger configures structured logging for the agent using log/slog.
//
// Design decisions:
//   - JSON output by default for log aggregation (journald, Loki, Datadog)
//   - Text output when LOG_FORMAT=text (local development)
//   - Level controlled by LOG_LEVEL env var (debug, info, warn, error)
//   - Structured fields: environment, phase, digest, duration included consistently
package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// contextKey is the key for the logger stored in context.Context.
type contextKey struct{}

// Setup configures the default global slog logger.
// Call once at startup before spawning goroutines.
func Setup() *slog.Logger {
	level := parseLevel(os.Getenv("LOG_LEVEL"))
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if strings.ToLower(os.Getenv("LOG_FORMAT")) == "text" {
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

// WithContainerName returns a child logger with the environment field pre-set.
// All log lines emitted from the reconciler for an environment will carry this.
func WithContainerName(logger *slog.Logger, containerName string) *slog.Logger {
	return logger.With("container", containerName)
}

// WithContext stores logger in ctx, enabling logger retrieval in deep call stacks.
func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, logger)
}

// FromContext retrieves the logger stored in ctx.
// Falls back to the default slog logger if none is stored.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(contextKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
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
	PhaseMigration   = "migration"
	PhaseRestart     = "restart"
	PhaseHealthCheck = "health_check"
	PhaseCommit      = "commit"
	PhaseRollback    = "rollback"
	PhaseNoop        = "noop"
)
