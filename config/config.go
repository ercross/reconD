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
	Workloads []Workload `yaml:"workloads"`
}

type Workload struct {
	Name            string      `yaml:"name"`
	Environment     string      `yaml:"environment"`
	ContainerName   string      `yaml:"container_name"`
	NotificationURL string      `yaml:"notification_url"`
	DeployCommand   string      `yaml:"deploy_command"`
	CheckInterval   Duration    `yaml:"check_interval"`
	StateDir        string      `yaml:"state_dir"`
	GitProvider     GitProvider `yaml:"git_provider"`
	HealthCheck     HealthCheck `yaml:"health_check"`
	Strategy        Strategy    `yaml:"strategy"`

	// Labels optionally scope docker image pruning to this workload's images.
	Labels map[string]string `yaml:"labels"`
}

type GitProvider struct {
	Owner string `yaml:"owner"`
	Repo  string `yaml:"repo"`
	// Token is optional; set via GITHUB_TOKEN env var for private repos.
	Token string `yaml:"token"`
}

type Strategy struct {
	Type        StrategyType `yaml:"type"`
	EnvFilePath string       `yaml:"env_file_path"`
	ImageTagKey string       `yaml:"image_tag_key"`
}

type StrategyType string

const (
	StrategyNone    StrategyType = "none"
	StrategyEnvFile StrategyType = "env_file"
)

type HealthCheck struct {
	Type     HealthCheckType `yaml:"type"`
	URL      string          `yaml:"url"`
	Retries  int             `yaml:"retries"`
	Interval Duration        `yaml:"interval"`
	Timeout  Duration        `yaml:"timeout"`
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
