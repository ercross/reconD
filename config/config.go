package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

type Config struct {
	Workloads []Workload `json:"workloads" yaml:"workloads"`
}

type Workload struct {
	Name            string            `json:"name" yaml:"name"`
	Environment     string            `json:"environment" yaml:"environment"`
	ContainerName   string            `json:"container_name" yaml:"container_name"`
	NotificationURL string            `json:"notification_url" yaml:"notification_url"`
	DeployCommand   string            `json:"deploy_command" yaml:"deploy_command"`
	CheckInterval   Duration          `json:"check_interval" yaml:"check_interval"`
	StateDir        string            `json:"state_dir" yaml:"state_dir"`
	GitProvider     GitProvider       `json:"git_provider" yaml:"git_provider"`
	HealthCheck     HealthCheckConfig `json:"health_check" yaml:"health_check"`
	Strategy        StrategyConfig    `json:"strategy" yaml:"strategy"`

	// Labels optionally scope docker image pruning to this workload's images.
	Labels map[string]string `json:"labels" yaml:"labels"`
}

type GitProvider struct {
	Owner string `json:"owner" yaml:"owner"`
	Repo  string `json:"repo" yaml:"repo"`
	// Token is optional; set via GITHUB_TOKEN env var for private repos.
	Token string `json:"token" yaml:"token"`
}

type StrategyConfig struct {
	Type        StrategyType `json:"type" yaml:"type"`
	EnvFilePath string       `json:"env_file_path" yaml:"env_file_path"`
	ImageTagKey string       `json:"image_tag_key" yaml:"image_tag_key"`
}

type StrategyType string

const (
	StrategyNone    StrategyType = "none"
	StrategyEnvFile StrategyType = "env_file"
)

type HealthCheckConfig struct {
	Type     HealthCheckType `json:"type" yaml:"type"`
	URL      string          `json:"url" yaml:"url"`
	Retries  int             `json:"retries" yaml:"retries"`
	Interval Duration        `json:"interval" yaml:"interval"`
	Timeout  Duration        `json:"timeout" yaml:"timeout"`
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

	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("error parsing config file %q: %w", path, err)
	}
	if len(cfg.Workloads) == 0 {
		var workload Workload
		if err := json.Unmarshal(raw, &workload); err != nil {
			return nil, fmt.Errorf("error parsing config file %q: %w", path, err)
		}
		if workload.Name != "" {
			cfg.Workloads = []Workload{workload}
		}
	}

	return &cfg, nil
}

func (w *Workload) fillDefaultOnZeroValues() {
	if w.CheckInterval.Duration < 10*time.Second {
		w.CheckInterval.Duration = 60 * time.Second
	}
	if w.HealthCheck.Retries == 0 {
		w.HealthCheck.Retries = 12
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
	if w.Environment == "" {
		return errors.New("environment is required")
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

func (h HealthCheckConfig) validate() error {
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

func (s StrategyConfig) validate() error {
	switch s.Type {
	case "", StrategyNone:
		return errors.New("strategy can not be empty")
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

func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		return d.setString(s)
	}
	var seconds int64
	if err := json.Unmarshal(data, &seconds); err != nil {
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
