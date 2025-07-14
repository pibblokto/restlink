package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type PodInfo struct {
	Name      string
	Namespace string
	Labels    map[string]string
	Cause     string // "PodCreation" or "ContainerRestart"
}

type SlackAlertConfig struct {
	TriggerName      string
	Channel          string
	Webhook          string
	Timestamp        string
	SourcePods       []PodInfo
	TargetPods       []PodInfo
	IncludeNamespace bool
	IncludeLabels    bool
	ShowMoreEnabled  bool
}

type Block map[string]any
type SlackMessage struct {
	Blocks []Block `json:"blocks"`
}

func SendSlackAlert(ctx context.Context, cfg SlackAlertConfig) error {
	var msg SlackMessage

	msg.Blocks = append(msg.Blocks, Block{"type": "divider"})

	msg.Blocks = append(msg.Blocks, Block{
		"type": "section",
		"text": map[string]string{
			"type": "mrkdwn",
			"text": fmt.Sprintf("*🚨 Alert: Restart Trigger `%s` fired at %s*", cfg.TriggerName, cfg.Timestamp),
		},
	})

	msg.Blocks = append(msg.Blocks, sourceTableSection("Source Pods That Triggered Restart", cfg.SourcePods, cfg.IncludeNamespace, cfg.IncludeLabels))
	msg.Blocks = append(msg.Blocks, targetTableSection("Target Pods That Were Restarted", cfg.TargetPods, cfg.IncludeNamespace, cfg.IncludeLabels))

	if cfg.ShowMoreEnabled {
		msg.Blocks = append(msg.Blocks, Block{
			"type": "section",
			"text": map[string]string{
				"type": "mrkdwn",
				"text": "_More details available in controller logs or dashboard..._",
			},
		})
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.Webhook, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create Slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Slack request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}

	return nil
}

func sourceTableSection(title string, pods []PodInfo, showNS, showLabels bool) Block {
	header := "| Name"
	if showNS {
		header += " | Namespace"
	}
	if showLabels {
		header += " | Labels"
	}
	header += " | Cause |\n"

	rows := ""
	for _, pod := range pods {
		row := fmt.Sprintf("| %s", pod.Name)
		if showNS {
			row += fmt.Sprintf(" | %s", pod.Namespace)
		}
		if showLabels {
			row += fmt.Sprintf(" | %v", pod.Labels)
		}
		row += fmt.Sprintf(" | %s |\n", pod.Cause)
		rows += row
	}

	return Block{
		"type": "section",
		"text": map[string]string{
			"type": "mrkdwn",
			"text": fmt.Sprintf("*%s*\n```%s%s```", title, header, rows),
		},
	}
}

func targetTableSection(title string, pods []PodInfo, showNS, showLabels bool) Block {
	header := "| Name"
	if showNS {
		header += " | Namespace"
	}
	if showLabels {
		header += " | Labels"
	}
	header += " |\n"

	rows := ""
	for _, pod := range pods {
		row := fmt.Sprintf("| %s", pod.Name)
		if showNS {
			row += fmt.Sprintf(" | %s", pod.Namespace)
		}
		if showLabels {
			row += fmt.Sprintf(" | %v", pod.Labels)
		}
		row += " |\n"
		rows += row
	}

	return Block{
		"type": "section",
		"text": map[string]string{
			"type": "mrkdwn",
			"text": fmt.Sprintf("*%s*\n```%s%s```", title, header, rows),
		},
	}
}
