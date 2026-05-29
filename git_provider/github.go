// Package git_provider provides a client for fetching deployment metadata
// published as GitHub Release assets.
//
// Uses google/go-github (v88) instead of a hand-rolled HTTP client.
// The primary motivation is DownloadReleaseAsset: GitHub asset downloads
// redirect to S3/blob storage, and the SDK handles that transparently when
// passed an http.Client for redirect following. Managing that redirect
// manually requires intercepting 302s, stripping auth headers before
// following to S3, and handling edge cases — maintenance we don't want.
package git_provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/v88/github"
	"golang.org/x/oauth2"
)

const (
	metadataAsset      = "deployment-metadata.json"
	defaultHTTPTimeout = 15 * time.Second
	assetSizeLimit     = 64 * 1024 // 64 KB
)

// githubClient fetches deployment metadata from GitHub Releases.
type githubClient struct {
	owner  string
	repo   string
	client *github.Client
}

// NewGithubClient constructs a githubClient for the given repository.
// token may be empty for public repositories (unauthenticated: 60 req/hr).
// For production use, always provide a token (authenticated: 5000 req/hr).
func NewGithubClient(owner, repo, token string) (GitProvider, error) {
	var httpClient *http.Client

	if token != "" {
		// oauth2.StaticTokenSource produces a transport that injects
		// Authorization: Bearer <token> on every request.
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		httpClient = oauth2.NewClient(context.Background(), ts)
		httpClient.Timeout = defaultHTTPTimeout
	}

	ghc, err := github.NewClient(github.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create new github client: %w", err)
	}

	return &githubClient{
		owner:  owner,
		repo:   repo,
		client: ghc,
	}, nil
}

// FetchLatestForEnvironment retrieves the most recent release whose tag
// starts with releasePrefix and contains a deployment-metadata.json asset
// matching the given environment.
func (c *githubClient) FetchLatestForEnvironment(ctx context.Context, releasePrefix, environment string) (meta DeploymentMetadata, err error) {
	releases, err := c.listReleases(ctx)
	if err != nil {
		return meta, fmt.Errorf("list releases: %w", err)
	}

	for _, r := range releases {
		if !strings.HasPrefix(r.GetTagName(), releasePrefix) {
			continue
		}

		asset := findMetadataAsset(r.Assets)
		if asset == nil {
			// Release exists but has no metadata asset — skip.
			// Can happen for manually created releases or legacy tags.
			continue
		}

		meta, err = c.downloadMetadata(ctx, asset.GetID())
		if err != nil {
			return meta, fmt.Errorf("download metadata for release %q: %w", r.GetTagName(), err)
		}

		// Verify environment matches — defence against misconfigured CI
		// publishing a production asset under a dev release tag.
		if meta.Environment != environment {
			continue
		}

		return meta, nil
	}

	return meta, ErrNoDeploymentMetaFound
}

// listReleases returns releases newest-first, capped at 30.
// 30 is sufficient: the reconciler only needs the latest release per prefix,
// and keeping it small bounds GitHub API response time.
func (c *githubClient) listReleases(ctx context.Context) ([]*github.RepositoryRelease, error) {
	releases, resp, err := c.client.Repositories.ListReleases(
		ctx,
		c.owner,
		c.repo,
		&github.ListOptions{PerPage: 30},
	)
	if err != nil {
		return nil, fmt.Errorf("GitHub ListReleases: %w", err)
	}
	defer resp.Body.Close()

	return releases, nil
}

// downloadMetadata fetches and parses the deployment-metadata.json asset
// identified by its numeric asset ID.
//
// DownloadReleaseAsset is the key reason for using the SDK:
//   - GitHub responds to asset download requests with a 302 redirect to S3.
//   - The redirect must be followed without the Authorization header (S3
//     rejects requests that contain both a pre-signed URL and an auth header).
//   - Passing http.DefaultClient as the followRedirectsClient tells the SDK
//     to follow the redirect using a plain unauthenticated client, which is
//     exactly what S3 expects.
//
// Previously this required manual redirect interception; the SDK handles it.
func (c *githubClient) downloadMetadata(ctx context.Context, assetID int64) (meta DeploymentMetadata, err error) {
	rc, _, err := c.client.Repositories.DownloadReleaseAsset(
		ctx,
		c.owner,
		c.repo,
		assetID,
		c.client.Client(), // documentation say to pass http.Client that performs authenticated request for private repo
	)
	if err != nil {
		return meta, fmt.Errorf("DownloadReleaseAsset %d: %w", assetID, err)
	}
	defer rc.Close()

	body, err := io.ReadAll(io.LimitReader(rc, assetSizeLimit))
	if err != nil {
		return meta, fmt.Errorf("read asset body: %w", err)
	}

	if err = json.Unmarshal(body, &meta); err != nil {
		return meta, fmt.Errorf("parse metadata JSON: %w", err)
	}
	return meta, nil
}

// findMetadataAsset locates the deployment-metadata.json asset in a release's
// asset list. Returns nil if not present.
func findMetadataAsset(assets []*github.ReleaseAsset) *github.ReleaseAsset {
	for _, a := range assets {
		if a.GetName() == metadataAsset {
			return a
		}
	}
	return nil
}
