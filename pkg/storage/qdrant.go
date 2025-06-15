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
	URL        string
	Client     *http.Client
	Collection string
}

// NewQdrantClient creates a new Qdrant client
func NewQdrantClient(url, collection string) *QdrantClient {
	return &QdrantClient{
		URL:        url,
		Client:     &http.Client{},
		Collection: collection,
	}
}

// StoreAlert implements the Storage interface
func (q *QdrantClient) StoreAlert(anomaly types.Anomaly, events []types.Event, vector []float32) error {
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
			Labels:      make(map[string]string),
			Events:      events,
			Metadata: map[string]interface{}{
				"detection_time": anomaly.Timestamp,
				"source":         "k8s-agent",
			},
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(alertVector)
	if err != nil {
		return fmt.Errorf("error marshaling alert vector: %v", err)
	}

	// Create point in Qdrant
	url := fmt.Sprintf("%s/collections/%s/points", q.URL, q.Collection)
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.Client.Do(req)
	if err != nil {
		return fmt.Errorf("error storing alert in Qdrant: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error response from Qdrant: %s - %s", resp.Status, string(body))
	}

	return nil
}

// SearchSimilarAlerts implements the Storage interface
func (q *QdrantClient) SearchSimilarAlerts(vector []float32, limit int) ([]AlertVector, error) {
	// Create search payload
	searchPayload := map[string]interface{}{
		"vector": vector,
		"limit":  limit,
	}

	data, err := json.Marshal(searchPayload)
	if err != nil {
		return nil, fmt.Errorf("error marshaling search payload: %v", err)
	}

	// Search in Qdrant
	url := fmt.Sprintf("%s/collections/%s/points/search", q.URL, q.Collection)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error searching alerts in Qdrant: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error response from Qdrant: %s - %s", resp.Status, string(body))
	}

	var result struct {
		Result []AlertVector `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding search response: %v", err)
	}

	return result.Result, nil
}

// GetAlert implements the Storage interface
func (q *QdrantClient) GetAlert(id string) (*AlertVector, error) {
	url := fmt.Sprintf("%s/collections/%s/points/%s", q.URL, q.Collection, id)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	resp, err := q.Client.Do(req)
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

	url := fmt.Sprintf("%s/collections/%s/points/scroll", q.URL, q.Collection)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.Client.Do(req)
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
	url := fmt.Sprintf("%s/collections/%s/points/%s", q.URL, q.Collection, id)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	resp, err := q.Client.Do(req)
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
