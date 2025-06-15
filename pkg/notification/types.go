package notification

import (
	"time"

	"github.com/rodgon/valkyrie/pkg/types"
)

// NotificationConfig represents the configuration for notifications
type NotificationConfig struct {
	Enabled     bool
	Type        string // "slack", "email", "webhook", "alertmanager"
	Endpoint    string
	Channel     string   // for Slack
	Recipients  []string // for email
	MinSeverity string
	// Alertmanager specific fields
	AlertManagerURL string
	AlertLabels     map[string]string
}

// Notifier interface for different notification methods
type Notifier interface {
	Notify(anomaly types.Anomaly) error
}

// AlertmanagerAlert represents an alert sent to Alertmanager
type AlertmanagerAlert struct {
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
}

// AlertmanagerPayload represents the payload sent to Alertmanager
type AlertmanagerPayload struct {
	Alerts []AlertmanagerAlert `json:"alerts"`
}
