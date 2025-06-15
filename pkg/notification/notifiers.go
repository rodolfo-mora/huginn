package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/rodgon/valkyrie/pkg/types"
)

// SlackNotifier implements Notifier for Slack
type SlackNotifier struct {
	WebhookURL string
	Channel    string
}

// EmailNotifier implements Notifier for email
type EmailNotifier struct {
	SMTPHost   string
	SMTPPort   int
	Username   string
	Password   string
	Recipients []string
}

// WebhookNotifier implements Notifier for webhooks
type WebhookNotifier struct {
	Endpoint string
}

// AlertmanagerNotifier implements Notifier for Prometheus Alertmanager
type AlertmanagerNotifier struct {
	URL           string
	DefaultLabels map[string]string
}

// Notify implements the Notifier interface for Slack
func (s *SlackNotifier) Notify(anomaly types.Anomaly) error {
	message := fmt.Sprintf("*%s Anomaly Detected*\nResource: %s\nSeverity: %s\nDescription: %s",
		anomaly.Type, anomaly.Resource, anomaly.Severity, anomaly.Description)

	// TODO: Implement actual Slack API call
	log.Printf("Would send to Slack: %s", message)
	return nil
}

// Notify implements the Notifier interface for Email
func (e *EmailNotifier) Notify(anomaly types.Anomaly) error {
	subject := fmt.Sprintf("Kubernetes Anomaly Alert: %s", anomaly.Type)
	body := fmt.Sprintf("Anomaly Details:\nResource: %s\nSeverity: %s\nDescription: %s",
		anomaly.Resource, anomaly.Severity, anomaly.Description)

	// TODO: Implement actual email sending
	log.Printf("Would send email: %s\n%s", subject, body)
	return nil
}

// Notify implements the Notifier interface for Webhook
func (w *WebhookNotifier) Notify(anomaly types.Anomaly) error {
	payload := map[string]interface{}{
		"type":        anomaly.Type,
		"resource":    anomaly.Resource,
		"severity":    anomaly.Severity,
		"description": anomaly.Description,
		"timestamp":   anomaly.Timestamp,
	}

	// TODO: Implement actual webhook call
	log.Printf("Would send webhook: %v", payload)
	return nil
}

// Notify implements the Notifier interface for Alertmanager
func (a *AlertmanagerNotifier) Notify(anomaly types.Anomaly) error {
	// Create alert labels
	labels := make(map[string]string)
	for k, v := range a.DefaultLabels {
		labels[k] = v
	}
	labels["alertname"] = fmt.Sprintf("k8s_%s_anomaly", anomaly.Type)
	labels["severity"] = anomaly.Severity
	labels["resource"] = anomaly.Resource
	if anomaly.Namespace != "" {
		labels["namespace"] = anomaly.Namespace
	}

	// Create alert annotations
	annotations := map[string]string{
		"description": anomaly.Description,
		"value":       fmt.Sprintf("%.2f", anomaly.Value),
		"threshold":   fmt.Sprintf("%.2f", anomaly.Threshold),
	}

	// Create alert
	alert := AlertmanagerAlert{
		Labels:       labels,
		Annotations:  annotations,
		StartsAt:     anomaly.Timestamp,
		EndsAt:       time.Now().Add(24 * time.Hour), // Alerts expire after 24 hours
		GeneratorURL: "k8s-agent",
	}

	// Create payload
	payload := AlertmanagerPayload{
		Alerts: []AlertmanagerAlert{alert},
	}

	// Marshal payload to JSON
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshaling alert payload: %v", err)
	}

	// Send alert to Alertmanager
	url := fmt.Sprintf("%s/api/v2/alerts", a.URL)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("error sending alert to Alertmanager: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error response from Alertmanager: %s - %s", resp.Status, string(body))
	}

	return nil
}
