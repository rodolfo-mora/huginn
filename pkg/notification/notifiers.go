package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rodgon/valkyrie/pkg/types"
)

// SlackNotifier implements notification via Slack
type SlackNotifier struct {
	WebhookURL string
}

// Notify sends an anomaly notification to Slack
func (n *SlackNotifier) Notify(anomaly types.Anomaly) error {
	message := fmt.Sprintf("*[%s] %s*\nResource: %s\nNamespace: %s\nSeverity: %s\nDescription: %s",
		anomaly.Type, anomaly.Resource, anomaly.Resource, anomaly.Namespace, anomaly.Severity, anomaly.Description)

	payload := map[string]string{
		"text": message,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack payload: %v", err)
	}

	resp, err := http.Post(n.WebhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send Slack notification: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Slack API returned non-200 status code: %d", resp.StatusCode)
	}

	return nil
}

// EmailNotifier implements notification via email
type EmailNotifier struct {
	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
	From         string
	To           []string
}

// Notify sends an anomaly notification via email
func (n *EmailNotifier) Notify(anomaly types.Anomaly) error {
	// TODO: Implement email sending
	fmt.Printf("Would send email notification for anomaly: %s\n", anomaly.Description)
	return nil
}

// WebhookNotifier implements notification via webhook
type WebhookNotifier struct {
	URL     string
	Headers map[string]string
}

// Notify sends an anomaly notification via webhook
func (n *WebhookNotifier) Notify(anomaly types.Anomaly) error {
	payload := map[string]interface{}{
		"type":        anomaly.Type,
		"resource":    anomaly.Resource,
		"namespace":   anomaly.Namespace,
		"severity":    anomaly.Severity,
		"description": anomaly.Description,
		"value":       anomaly.Value,
		"threshold":   anomaly.Threshold,
		"timestamp":   anomaly.Timestamp,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %v", err)
	}

	req, err := http.NewRequest("POST", n.URL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range n.Headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook notification: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook endpoint returned non-2xx status code: %d", resp.StatusCode)
	}

	return nil
}

// AlertmanagerNotifier implements notification via Alertmanager
type AlertmanagerNotifier struct {
	URL           string
	DefaultLabels map[string]string
}

// Notify sends an anomaly notification to Alertmanager
func (n *AlertmanagerNotifier) Notify(anomaly types.Anomaly) error {
	labels := make(map[string]string)
	for k, v := range n.DefaultLabels {
		labels[k] = v
	}
	labels["alertname"] = anomaly.Type
	labels["resource"] = anomaly.Resource
	labels["namespace"] = anomaly.Namespace
	labels["severity"] = anomaly.Severity

	annotations := map[string]string{
		"description": anomaly.Description,
		"value":       fmt.Sprintf("%.2f", anomaly.Value),
		"threshold":   fmt.Sprintf("%.2f", anomaly.Threshold),
	}

	alert := types.AlertmanagerAlert{
		Labels:       labels,
		Annotations:  annotations,
		StartsAt:     anomaly.Timestamp,
		EndsAt:       time.Now().Add(24 * time.Hour), // Alerts expire after 24 hours
		GeneratorURL: "https://github.com/rodgon/valkyrie",
	}

	payload := types.AlertmanagerPayload{
		Alerts: []types.AlertmanagerAlert{alert},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Alertmanager payload: %v", err)
	}

	resp, err := http.Post(n.URL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send Alertmanager notification: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Alertmanager API returned non-200 status code: %d", resp.StatusCode)
	}

	return nil
}
