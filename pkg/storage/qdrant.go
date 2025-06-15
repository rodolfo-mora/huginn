package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rodgon/valkyrie/pkg/types"
)

// QdrantClient implements the Storage interface using Qdrant
type QdrantClient struct {
	url        string
	collection string
	client     *http.Client
}

// NewQdrantClient creates a new Qdrant client
func NewQdrantClient(url, collection string) (*QdrantClient, error) {
	return &QdrantClient{
		url:        url,
		collection: collection,
		client:     &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// StoreAlert stores an alert in Qdrant
func (c *QdrantClient) StoreAlert(vector []float32, anomaly types.Anomaly) error {
	// Create alert vector
	alertVector := AlertVector{
		ID:        fmt.Sprintf("%s-%s-%d", anomaly.Type, anomaly.Resource, time.Now().UnixNano()),
		Vector:    vector,
		Timestamp: time.Now(),
		Payload: AlertVectorPayload{
			Type:        anomaly.Type,
			Resource:    anomaly.Resource,
			Namespace:   anomaly.Namespace,
			Severity:    anomaly.Severity,
			Description: anomaly.Description,
			Value:       anomaly.Value,
			Threshold:   anomaly.Threshold,
			Labels:      anomaly.Labels,
			Events:      anomaly.Events,
			Metadata:    anomaly.Metadata,
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(alertVector)
	if err != nil {
		return fmt.Errorf("failed to marshal alert vector: %v", err)
	}

	// Send to Qdrant
	url := fmt.Sprintf("%s/collections/%s/points", c.url, c.collection)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Qdrant API returned non-200 status code: %d", resp.StatusCode)
	}

	return nil
}

// SearchSimilarAlerts searches for similar alerts in Qdrant
func (c *QdrantClient) SearchSimilarAlerts(vector []float32, limit int) ([]types.Anomaly, error) {
	// Create search payload
	payload := map[string]interface{}{
		"vector": vector,
		"limit":  limit,
	}

	// Marshal to JSON
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search payload: %v", err)
	}

	// Send to Qdrant
	url := fmt.Sprintf("%s/collections/%s/search", c.url, c.collection)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Qdrant API returned non-200 status code: %d", resp.StatusCode)
	}

	// Decode response
	var result struct {
		Results []struct {
			Payload AlertVectorPayload `json:"payload"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	// Convert to anomalies
	anomalies := make([]types.Anomaly, len(result.Results))
	for i, r := range result.Results {
		anomalies[i] = types.Anomaly{
			Type:        r.Payload.Type,
			Resource:    r.Payload.Resource,
			Namespace:   r.Payload.Namespace,
			Severity:    r.Payload.Severity,
			Description: r.Payload.Description,
			Value:       r.Payload.Value,
			Threshold:   r.Payload.Threshold,
			Labels:      r.Payload.Labels,
			Events:      r.Payload.Events,
			Metadata:    r.Payload.Metadata,
		}
	}

	return anomalies, nil
}

// GetAlert implements the Storage interface
func (q *QdrantClient) GetAlert(id string) (*AlertVector, error) {
	url := fmt.Sprintf("%s/collections/%s/points/%s", q.url, q.collection, id)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	resp, err := q.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error getting alert from Qdrant: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error response from Qdrant: %s - %s", resp.Status, string(body))
	}

	var result AlertVector
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding alert response: %v", err)
	}

	return &result, nil
}

// ListAlerts implements the Storage interface
func (q *QdrantClient) ListAlerts(namespace, severity string, startTime, endTime time.Time) ([]AlertVector, error) {
	// Create filter payload
	filter := map[string]interface{}{
		"must": []map[string]interface{}{},
	}

	if namespace != "" {
		filter["must"] = append(filter["must"].([]map[string]interface{}), map[string]interface{}{
			"key":   "namespace",
			"match": map[string]string{"value": namespace},
		})
	}

	if severity != "" {
		filter["must"] = append(filter["must"].([]map[string]interface{}), map[string]interface{}{
			"key":   "severity",
			"match": map[string]string{"value": severity},
		})
	}

	// Add time range filter
	filter["must"] = append(filter["must"].([]map[string]interface{}), map[string]interface{}{
		"range": map[string]interface{}{
			"timestamp": map[string]interface{}{
				"gte": startTime.Unix(),
				"lte": endTime.Unix(),
			},
		},
	})

	// Create scroll request
	scrollPayload := map[string]interface{}{
		"filter": filter,
		"limit":  100,
	}

	data, err := json.Marshal(scrollPayload)
	if err != nil {
		return nil, fmt.Errorf("error marshaling scroll payload: %v", err)
	}

	url := fmt.Sprintf("%s/collections/%s/points/scroll", q.url, q.collection)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error listing alerts from Qdrant: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error response from Qdrant: %s - %s", resp.Status, string(body))
	}

	var result struct {
		Points []AlertVector `json:"points"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding list response: %v", err)
	}

	return result.Points, nil
}

// DeleteAlert implements the Storage interface
func (q *QdrantClient) DeleteAlert(id string) error {
	url := fmt.Sprintf("%s/collections/%s/points/%s", q.url, q.collection, id)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("error deleting alert from Qdrant: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error response from Qdrant: %s - %s", resp.Status, string(body))
	}

	return nil
}
