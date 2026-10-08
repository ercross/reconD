// Package config loads and validates reconD workload configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// Log is optional and controls reconD's process-level logging output.
	Log Log `yaml:"log"`

	// Workloads is the required list of container workloads reconD should watch
	// and reconcile. Configure one entry for each independently deployed
	// service.
	Workloads []Workload `yaml:"workloads"`
}

type Log struct {
	// Format is optional and controls the log handler format. Set it to "text"
	// for local development; any other value uses structured JSON output.
	Format string `yaml:"format"`

	// Level is optional and controls the minimum log level. Supported values are
	// "debug", "info", "warn", and "error"; empty defaults to "info".
	Level string `yaml:"level"`
}

// Workload is a standalone unit of deployment (e.g., a container) reconD can watch and reconcile
type Workload struct {
	// Name is required and identifies this workload in logs, notifications, and
	// persisted deployment state.
	Name string `yaml:"name"`

	// ReleasePrefix is optional and scopes GitHub release selection by tag
	// prefix. For example, "api-prod" matches release tags like
	// "api-prod-sha-10a3e42". Leave it empty only when this repository publishes
	// one deployable artifact stream or when the newest valid metadata release is
	// always the desired workload.
	ReleasePrefix string `yaml:"release_prefix"`

	// ContainerName is required and must match the Docker container reconD
	// inspects to determine the workload's current runtime state.
	ContainerName string `yaml:"container_name"`

	// NotificationURL is optional. When set, reconD sends deployment events to
	// this Slack webhook URL; when empty, notifications are disabled.
	NotificationURL string `yaml:"notification_url"`

	// DeployCommand is required. reconD runs it with /bin/sh after applying the
	// deployment strategy, passing workload and image metadata in WORKLOAD_*
	// environment variables.
	DeployCommand string `yaml:"deploy_command"`

	// CheckInterval is optional and controls how often reconD checks for new
	// deployment metadata. Values may be Go duration strings like "60s" or a
	// number of seconds; values below 10s default to 60s.
	CheckInterval Duration `yaml:"check_interval"`

	// StateDir is required and points to the local directory where reconD stores
	// deployed and rollback state for this workload.
	StateDir string `yaml:"state_dir"`

	// GitProvider is required and tells reconD which repository to read
	// deployment metadata from.
	GitProvider GitProvider `yaml:"git_provider"`

	// HealthCheck is optional. When configured, reconD waits for the workload to
	// become healthy after deploy and rollback commands; a failed check triggers
	// rollback when previous state is available.
	HealthCheck HealthCheck `yaml:"health_check"`

	// Strategy is optional. Use it when deployment metadata must be written
	// somewhere before DeployCommand runs, such as updating an env file with the
	// image tag.
	Strategy Strategy `yaml:"strategy"`

	// Labels is optional and scopes Docker image pruning to images with these
	// labels after a successful deployment.
	Labels map[string]string `yaml:"labels"`
}

type GitProvider struct {
	// Owner is required and names the GitHub owner or organization containing
	// the deployment metadata repository.
	Owner string `yaml:"owner"`

	// Repo is required and names the GitHub repository containing deployment
	// metadata for this workload.
	Repo string `yaml:"repo"`

	// Token is optional and is used to authenticate GitHub API requests for
	// private repositories. If omitted, reconD reads GITHUB_TOKEN from the
	// process environment.
	Token string `yaml:"token"`
}

type Strategy struct {
	// Type is optional. Leave it empty or set it to "none" to run no strategy;
	// set it to "env_file" to write deployment metadata into an env file before
	// DeployCommand runs.
	Type StrategyType `yaml:"type"`

	// EnvFilePath is required when Type is "env_file" and ignored otherwise. It
	// points to the env file reconD should create or update.
	EnvFilePath string `yaml:"env_file_path"`

	// ImageTagKey is optional for the "env_file" strategy and defaults to
	// "IMAGE_TAG". It is the key reconD writes with the selected image tag.
	ImageTagKey string `yaml:"image_tag_key"`
}

type StrategyType string

const (
	StrategyNone    StrategyType = "none"
	StrategyEnvFile StrategyType = "env_file"
)

type HealthCheck struct {
	// Type is optional. Leave it empty or set it to "none" to skip health
	// checks; set it to "http" or "tcp" to poll URL after deployment.
	Type HealthCheckType `yaml:"type"`

	// URL is required when Type is "http" or "tcp" and ignored otherwise. Use
	// an HTTP URL for HTTP checks or a tcp://host:port URL for TCP checks.
	URL string `yaml:"url"`

	// Retries is optional and defaults to 12. It controls how many health check
	// attempts reconD makes before treating the deployment as unhealthy.
	Retries int `yaml:"retries"`

	// Interval is optional and defaults to 10s. It controls how long reconD waits
	// between health check attempts.
	Interval Duration `yaml:"interval"`

	// Timeout is optional and defaults to 5s. It controls the per-attempt timeout
	// for HTTP requests and TCP connections.
	Timeout Duration `yaml:"timeout"`
}

type HealthCheckType string

const (
	HealthCheckNone    HealthCheckType = "none"
	HealthCheckHTTP    HealthCheckType = "http"
	HealthCheckTCP     HealthCheckType = "tcp"
	HealthCheckCommand HealthCheckType = "command"
)

type Duration struct {
	time.Duration
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file %q: %w", path, err)
	}

	cfg, err := parseConfig(path, raw)
	if err != nil {
		return nil, err
	}

	for i := range cfg.Workloads {
		cfg.Workloads[i].fillDefaultOnZeroValues()
	}

	return cfg, cfg.validate()
}

func parseConfig(path string, raw []byte) (*Config, error) {
	var cfg Config

	if err := unmarshalConfig(path, raw, &cfg); err != nil {
		return nil, fmt.Errorf("error parsing config file %q: %w", path, err)
	}

	return &cfg, nil
}

func unmarshalConfig(path string, raw []byte, v any) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return yaml.Unmarshal(raw, v)
	default:
		return fmt.Errorf("unsupported config file extension %q; only .yaml and .yml are supported", filepath.Ext(path))
	}
}

func (w *Workload) fillDefaultOnZeroValues() {
	if w.CheckInterval.Duration < 10*time.Second {
		w.CheckInterval.Duration = 60 * time.Second
	}
	if w.HealthCheck.Retries == 0 {
		w.HealthCheck.Retries = 12
	}

	if w.GitProvider.Token == "" {
		w.GitProvider.Token = os.Getenv("GITHUB_TOKEN")
	}

	if w.HealthCheck.Interval.Duration == 0 {
		w.HealthCheck.Interval.Duration = 10 * time.Second
	}
	if w.HealthCheck.Timeout.Duration == 0 {
		w.HealthCheck.Timeout.Duration = 5 * time.Second
	}
	if w.Strategy.Type == StrategyEnvFile && w.Strategy.ImageTagKey == "" {
		w.Strategy.ImageTagKey = "IMAGE_TAG"
	}
}

func (cfg Config) validate() error {
	if len(cfg.Workloads) == 0 {
		return fmt.Errorf("at least one workload must be configured")
	}

	for i, workload := range cfg.Workloads {
		if err := workload.validate(); err != nil {
			return fmt.Errorf("workload %d: %w", i, err)
		}
	}
	return nil
}

func (w Workload) validate() error {
	if w.Name == "" {
		return errors.New("name is required")
	}
	if w.ContainerName == "" {
		return errors.New("container_name is required")
	}
	if w.DeployCommand == "" {
		return errors.New("deploy_command is required")
	}
	if w.StateDir == "" {
		return errors.New("state_dir is required")
	}
	if w.GitProvider.Owner == "" {
		return errors.New("git_provider.owner is required")
	}
	if w.GitProvider.Repo == "" {
		return errors.New("git_provider.repo is required")
	}
	if err := w.HealthCheck.validate(); err != nil {
		return err
	}
	return w.Strategy.validate()
}

func (h HealthCheck) validate() error {
	switch h.Type {
	case "", HealthCheckNone:
		return nil
	case HealthCheckHTTP, HealthCheckTCP:
		if h.URL == "" {
			return errors.New("health_check.url is required")
		}
	case HealthCheckCommand:
		return errors.New("health_check.type command is recognized but not implemented")
	default:
		return fmt.Errorf("unsupported health_check.type %q", h.Type)
	}
	return nil
}

func (s Strategy) validate() error {
	switch s.Type {
	case "", StrategyNone:
		return nil
	case StrategyEnvFile:
		if s.EnvFilePath == "" {
			return errors.New("strategy.env_file_path is required for env_file strategy")
		}
		if s.ImageTagKey == "" {
			return errors.New("strategy.image_tag_key is required for env_file strategy")
		}
	default:
		return fmt.Errorf("unsupported strategy.type %q", s.Type)
	}
	return nil
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value.ShortTag() == "!!str" {
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		return d.setString(s)
	}

	var seconds int64
	if err := value.Decode(&seconds); err != nil {
		return err
	}
	d.Duration = time.Duration(seconds) * time.Second
	return nil
}

func (d *Duration) setString(value string) error {
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", value, err)
	}
	d.Duration = parsed
	return nil
}
