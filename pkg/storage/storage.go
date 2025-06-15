package storage

import (
	"time"

	"github.com/rodgon/valkyrie/pkg/types"
)

// Storage defines the interface for alert storage
type Storage interface {
	// StoreAlert stores an alert with its vector embedding
	StoreAlert(vector []float32, anomaly types.Anomaly) error

	// SearchSimilarAlerts searches for similar alerts using vector similarity
	SearchSimilarAlerts(vector []float32, limit int) ([]types.Anomaly, error)
}

// AlertVector represents an alert stored in the vector database
type AlertVector struct {
	ID        string             `json:"id"`
	Vector    []float32          `json:"vector"`
	Payload   AlertVectorPayload `json:"payload"`
	Timestamp time.Time          `json:"timestamp"`
}

// AlertVectorPayload represents the payload stored with an alert vector
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
