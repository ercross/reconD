// Package health implements post-deployment health checking.
// After each deployment, the agent polls the application's health endpoint
// to verify the new version started successfully before committing state.
//
// A failed health check triggers automatic rollback.
package health

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Checker polls an HTTP health endpoint.
type Checker struct {
	url    string
	client *http.Client
}

// NewChecker creates a Checker for the given URL with per-request timeout.
func NewChecker(url string, requestTimeout time.Duration) *Checker {
	return &Checker{
		url: url,
		client: &http.Client{
			Timeout: requestTimeout,
			// Do not follow redirects — a redirect on /health is suspicious.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Check performs a single health check. It returns nil if the endpoint
// responds with HTTP 200, or an error describing the failure.
func (c *Checker) Check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", c.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check %s: HTTP %d", c.url, resp.StatusCode)
	}
	return nil
}

// WaitHealthy polls the health endpoint until it returns HTTP 200
// or the retry budget is exhausted.
//
// Parameters:
//   - retries: total number of attempts
//   - interval: fixed wait between attempts
//   - timeout: per-request HTTP timeout
//
// Returns nil on success, error after all retries fail.
func WaitHealthy(ctx context.Context, url string, retries int, interval, timeout time.Duration) error {
	checker := NewChecker(url, timeout)

	for attempt := 1; attempt <= retries; attempt++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled: %w", err)
		}

		err := checker.Check(ctx)
		if err == nil {
			return nil // healthy
		}

		if attempt == retries {
			return fmt.Errorf("unhealthy after %d attempts: %w", retries, err)
		}

		// Wait before next attempt, respecting context cancellation.
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during health wait: %w", ctx.Err())
		case <-time.After(interval):
		}
	}

	return fmt.Errorf("no attempts made")
}
