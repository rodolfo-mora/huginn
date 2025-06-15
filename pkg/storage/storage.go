package storage

import (
	"time"

	"github.com/rodgon/valkyrie/pkg/types"
)

// Storage defines the interface for alert storage implementations
type Storage interface {
	// StoreAlert stores an alert with its associated events and vector embedding
	StoreAlert(anomaly types.Anomaly, events []types.Event, vector []float32) error

	// SearchSimilarAlerts searches for similar alerts using vector similarity
	SearchSimilarAlerts(vector []float32, limit int) ([]AlertVector, error)

	// GetAlert retrieves an alert by its ID
	GetAlert(id string) (*AlertVector, error)

	// ListAlerts retrieves alerts with optional filtering
	ListAlerts(namespace, severity string, startTime, endTime time.Time) ([]AlertVector, error)

	// DeleteAlert deletes an alert by its ID
	DeleteAlert(id string) error
}

// AlertVector represents an alert stored in any storage backend
type AlertVector struct {
	ID        string             `json:"id"`
	Vector    []float32          `json:"vector"`
	Payload   AlertVectorPayload `json:"payload"`
	Timestamp time.Time          `json:"timestamp"`
}

// AlertVectorPayload contains the alert data and metadata
type AlertVectorPayload struct {
	Type        string                 `json:"type"`
	Resource    string                 `json:"resource"`
	Namespace   string                 `json:"namespace"`
	Severity    string                 `json:"severity"`
	Description string                 `json:"description"`
	Value       float64                `json:"value"`
	Threshold   float64                `json:"threshold"`
	Labels      map[string]string      `json:"labels"`
	Events      []types.Event          `json:"events"`
	Metadata    map[string]interface{} `json:"metadata"`
}
