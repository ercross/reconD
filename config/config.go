package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

type Config struct {
	Services []Service `json:"services"`
}

type Service struct {
	// Labels baked into the image via Dockerfile
	Labels map[string]string `json:"labels" validate:"required,min=1"`

	// Name of this service as written in the docker compose configuration
	Name            string        `json:"name"`
	Environment     string        `json:"environment"`
	ContainerName   string        `json:"container_name"`
	NotificationUrl string        `json:"notification_url"`
	Reconciler      *Reconciler   `json:"reconciler"`
	Orchestration   Orchestration `json:"orchestration"`
}

type Orchestration struct {
	StartCommand string `json:"start"`
	StopCommand  string `json:"stop"`
}

type Reconciler struct {
	// PollIntervalInSeconds controls how frequently each environment reconciler checks
	// for new deployment metadata. Default: 60.
	PollIntervalInSeconds int64             `json:"poll_interval_in_seconds"`
	GitProvider           GitProvider       `json:"git_provider"`
	ReleasePrefix         string            `json:"release_prefix"`
	StateDir              string            `json:"state_dir"`
	HealthCheckURL        string            `json:"health_check_url"`
	HealthCheck           HealthCheckConfig `json:"health_check"`
}

type GitProvider struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	// Token is optional; set via GITHUB_TOKEN env var for private repos.
	// If empty, unauthenticated requests are used (60 req/hr limit applies).
	Token string `json:"token"`
}

type HealthCheckConfig struct {
	// MaxRetries is the number of health check attempts before declaring failure.
	MaxRetries int `json:"retries"`

	// Interval is the wait between retries.
	Interval time.Duration

	// Timeout is the per-request HTTP timeout.
	Timeout time.Duration
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file %q: %w", path, err)
	}

	var cfg Config
	if err = json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("error parsing config file %q: %w", path, err)
	}

	for i := range cfg.Services {
		cfg.Services[i].fillDefaultOnZeroValues()
	}

	return &cfg, cfg.validate()
}

func (s *Service) fillDefaultOnZeroValues() {

	if s.Reconciler.PollIntervalInSeconds < 10 {
		s.Reconciler.PollIntervalInSeconds = 60
	}
	if s.Reconciler.HealthCheck.MaxRetries == 0 {
		s.Reconciler.HealthCheck.MaxRetries = 12
	}
	if s.Reconciler.HealthCheck.Interval == 0 {
		s.Reconciler.HealthCheck.Interval = 10 * time.Second
	}
	if s.Reconciler.HealthCheck.Timeout == 0 {
		s.Reconciler.HealthCheck.Timeout = 5 * time.Second
	}
}

func (cfg Config) validate() error {
	if len(cfg.Services) == 0 {
		return fmt.Errorf("at least one service must be configured")
	}

	for i, service := range cfg.Services {
		if err := service.validate(); err != nil {
			return fmt.Errorf("service %d: %w", i, err)
		}
	}
	return nil
}

func (r Reconciler) validate() error {
	if r.ReleasePrefix == "" {
		return errors.New("release_prefix is required")
	}
	if r.StateDir == "" {
		return errors.New("state_dir is required")
	}
	if r.HealthCheckURL == "" {
		return errors.New("health_check_url is required")
	}
	if r.GitProvider.Owner == "" {
		return errors.New("git_provider.owner is required")
	}
	if r.GitProvider.Repo == "" {
		return errors.New("git_provider.repo is required")
	}
	return nil
}

func (s Service) validate() error {
	if s.Name == "" {
		return errors.New("service_name is required")
	}

	if s.Environment != "" {
		return errors.New("image_prefix is required")
	}

	if s.ContainerName == "" {
		return errors.New("container name is required")
	}

	if len(s.Labels) == 0 {
		return errors.New("at least one image label is required")
	}

	if s.NotificationUrl == "" {
		return errors.New("notification_url is required")
	}

	if s.Orchestration.StartCommand == "" {
		return errors.New("start command is required")
	}

	if s.Orchestration.StopCommand == "" {
		return errors.New("stop command is required")
	}

	return s.Reconciler.validate()
}
