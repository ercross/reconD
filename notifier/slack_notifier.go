package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/ercross/reconD/git_provider"
	"github.com/ercross/reconD/state"
)

var defaultHTTPTimeout = 15 * time.Second

type Slack struct {
	url  string
	log  *slog.Logger
	http *http.Client
}

type deploymentState string

const (
	deploymentStateSuccess    deploymentState = "success"
	deploymentStateFailed     deploymentState = "failed"
	deploymentStateInitialize deploymentState = "initialize"
)

type slackNotification struct {
	State          deploymentState `json:"state"`
	Workload       string          `json:"workload"`
	Image          string          `json:"image"`
	ImageTag       string          `json:"image_tag"`
	ManifestDigest string          `json:"manifest_digest"`
	GitSHA         string          `json:"git_sha"`
	RollbackFrom   string          `json:"rollback_from,omitempty"`
	Error          string
}

func NewSlackNotifier(url string, log *slog.Logger) *Slack {
	return &Slack{
		url:  url,
		log:  log,
		http: &http.Client{Timeout: defaultHTTPTimeout},
	}
}

func (sl *Slack) NotifyOnNewDeploymentStarted(workloadName string, meta git_provider.DeploymentMetadata) {
	msg := slackNotificationFromDeploymentMeta(workloadName, meta, deploymentStateInitialize)
	err := sl.send(msg)
	if err != nil {
		sl.log.Error("failed to send slack notification", "error", err)
	}
}
func (sl *Slack) NotifyOnDeploymentFailed(workloadName string, meta git_provider.DeploymentMetadata, err error) {
	msg := slackNotificationFromDeploymentMeta(workloadName, meta, deploymentStateFailed)
	msg.Error = err.Error()
	err = sl.send(msg)
	if err != nil {
		sl.log.Error("Failed to send slack notification", "error", err)
	}
}

func (sl *Slack) NotifyOnDeploymentSuccess(dep state.DeploymentState) {
	msg := slackNotificationFromDeploymentState(dep, deploymentStateSuccess)
	err := sl.send(msg)
	if err != nil {
		sl.log.Error("Failed to send slack notification", "error", err)
	}
}

func slackNotificationFromDeploymentMeta(workloadName string, meta git_provider.DeploymentMetadata, state deploymentState) slackNotification {
	return slackNotification{
		State:          state,
		Workload:       workloadName,
		Image:          meta.Image,
		ImageTag:       meta.ImageTag,
		ManifestDigest: meta.ManifestDigest,
		GitSHA:         meta.GitSHA,
		RollbackFrom:   "<none>",
	}
}

func slackNotificationFromDeploymentState(depState state.DeploymentState, st deploymentState) slackNotification {
	return slackNotification{
		State:          st,
		Workload:       depState.WorkloadName,
		Image:          depState.Image,
		ImageTag:       depState.ImageTag,
		ManifestDigest: depState.ManifestDigest,
		GitSHA:         depState.GitSHA,
		RollbackFrom:   depState.RollbackFrom,
	}
}

func (sl *Slack) send(notification slackNotification) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	payload := buildSlackMessage(notification)

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sl.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to initialize new request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := sl.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("status not successful HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

type slackBlock struct {
	Type   string      `json:"type"`
	Text   *slackText  `json:"text,omitempty"`
	Fields []slackText `json:"fields,omitempty"`
}

type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func buildSlackMessage(msg slackNotification) map[string]interface{} {

	var lagosTime string
	location, err := time.LoadLocation("Africa/Lagos")
	if err != nil {
		location = time.UTC
	}
	lagosTime = time.Now().In(location).Format("Monday, 02 January 2006 03:04 pm")

	return map[string]interface{}{
		"text": fmt.Sprintf(
			"Deployment for %s",
			msg.Workload,
		),
		"blocks": []slackBlock{
			{
				Type: "header",
				Text: &slackText{
					Type: "plain_text",
					Text: fmt.Sprintf("Workload %s", msg.Workload),
				},
			},
			{
				Type: "section",
				Text: &slackText{
					Type: "plain_text",
					Text: fmt.Sprintf("State: %s", msg.State),
				},
			},
			{
				Type: "section",
				Text: &slackText{
					Type: "plain_text",
					Text: fmt.Sprintf("Error: %s", msg.Error),
				},
			},
			{
				Type: "section",
				Text: &slackText{
					Type: "plain_text",
					Text: fmt.Sprintf("Digest: %s", msg.ManifestDigest),
				},
			},
			{
				Type: "section",
				Text: &slackText{
					Type: "plain_text",
					Text: fmt.Sprintf("Image: %s", msg.Image),
				},
			},
			{
				Type: "section",
				Text: &slackText{
					Type: "plain_text",
					Text: fmt.Sprintf("Tag: %s", msg.ImageTag),
				},
			},
			{
				Type: "section",
				Text: &slackText{
					Type: "plain_text",
					Text: fmt.Sprintf("Rollback From: %s", msg.RollbackFrom),
				},
			},
			{
				Type: "section",
				Text: &slackText{
					Type: "plain_text",
					Text: fmt.Sprintf("Timestamp: %s", lagosTime),
				},
			},
			{
				Type: "divider",
			},
		},
	}
}
