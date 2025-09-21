package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rodolfo-mora/huginn/pkg/types"
)

// QdrantClient implements the Storage interface using Qdrant
type QdrantClient struct {
	url        string
	collection string
	client     *http.Client
	vectorSize int
	distance   string
}

// NewQdrantClient creates a new Qdrant client
func NewQdrantClient(url, collection string, vectorSize int, distance string) (*QdrantClient, error) {
	client := &QdrantClient{
		url:        url,
		collection: collection,
		client:     &http.Client{Timeout: 10 * time.Second},
		vectorSize: vectorSize,
		distance:   distance,
	}

	// Ensure collection exists
	if err := client.ensureCollection(); err != nil {
		return nil, fmt.Errorf("failed to ensure collection exists: %v", err)
	}

	return client, nil
}

// ensureCollection ensures the collection exists with proper configuration
func (c *QdrantClient) ensureCollection() error {
	// First, check if collection exists
	url := fmt.Sprintf("%s/collections/%s", c.url, c.collection)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to check collection: %v", err)
	}
	defer resp.Body.Close()

	// If collection exists, we're done
	if resp.StatusCode == http.StatusOK {
		return nil
	}

	// If collection doesn't exist, create it
	if resp.StatusCode == http.StatusNotFound {
		return c.createCollection()
	}

	// Unexpected status code
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("unexpected status code %d when checking collection: %s", resp.StatusCode, string(body))
}

// createCollection creates a new collection with proper configuration
func (c *QdrantClient) createCollection() error {
	// Defaults if not provided
	size := c.vectorSize
	if size <= 0 {
		size = 384
	}
	distance := c.distance
	if distance == "" {
		distance = "Cosine"
	}
	// Normalize distance casing to what Qdrant expects
	switch strings.ToLower(distance) {
	case "cosine":
		distance = "Cosine"
	case "euclid", "euclidean", "l2":
		distance = "Euclid"
	case "dot", "dotproduct":
		distance = "Dot"
	}

	// Create collection configuration
	config := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     size,
			"distance": distance,
		},
	}

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal collection config: %v", err)
	}

	url := fmt.Sprintf("%s/collections/%s", c.url, c.collection)
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create collection: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create collection, status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// StoreAlert stores an alert in Qdrant
func (c *QdrantClient) StoreAlert(vector []float32, anomaly types.Anomaly) error {
	// Create the point payload in Qdrant format
	point := map[string]interface{}{
		"id":     uuid.New().String(),
		"vector": vector,
		"payload": map[string]interface{}{
			"type":                 anomaly.Type,
			"resourcetype":         anomaly.ResourceType,
			"resource":             anomaly.Resource,
			"cluster":              anomaly.ClusterName,
			"namespace":            anomaly.Namespace,
			"nodename":             anomaly.NodeName,
			"severity":             anomaly.Severity,
			"description":          anomaly.Description,
			"value":                anomaly.Value,
			"threshold":            anomaly.Threshold,
			"namespacesonthisnode": anomaly.NamespacesOnThisNode,
			"events":               anomaly.Events,
			"labels":               anomaly.Labels,
			"timestamp":            time.Now().Unix(),
		},
	}

	// Add optional fields if they exist
	if anomaly.Labels != nil {
		point["payload"].(map[string]interface{})["labels"] = anomaly.Labels
	}
	if anomaly.Events != nil {
		point["payload"].(map[string]interface{})["events"] = anomaly.Events
	}
	if anomaly.Metadata != nil {
		point["payload"].(map[string]interface{})["metadata"] = anomaly.Metadata
	}

	// Create the upsert payload
	upsertPayload := map[string]interface{}{
		"points": []map[string]interface{}{point},
	}

	// Marshal to JSON
	data, err := json.Marshal(upsertPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal upsert payload: %v", err)
	}

	// Send to Qdrant
	url := fmt.Sprintf("%s/collections/%s/points", c.url, c.collection)
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant api returned status code %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// SearchSimilarAlerts searches for similar alerts in Qdrant
func (c *QdrantClient) SearchSimilarAlerts(vector []float32, limit int) ([]types.Anomaly, error) {
	// Create search payload in Qdrant format
	searchPayload := map[string]interface{}{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
	}

	// Marshal to JSON
	data, err := json.Marshal(searchPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search payload: %v", err)
	}

	// Send to Qdrant
	url := fmt.Sprintf("%s/collections/%s/points/search", c.url, c.collection)
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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("qdrant api returned status code %d: %s", resp.StatusCode, string(body))
	}

	// Decode response
	var result struct {
		Result []struct {
			Payload map[string]interface{} `json:"payload"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	// Convert to anomalies with consistent field handling
	anomalies := make([]types.Anomaly, len(result.Result))
	for i, r := range result.Result {
		payload := r.Payload

		anomaly := types.Anomaly{
			Type:                 getStringFromPayload(payload, "type"),
			ResourceType:         getStringFromPayload(payload, "resourcetype"),
			Resource:             getStringFromPayload(payload, "resource"),
			ClusterName:          getStringFromPayload(payload, "cluster"),
			Namespace:            getStringFromPayload(payload, "namespace"),
			NodeName:             getStringFromPayload(payload, "nodename"),
			Severity:             getStringFromPayload(payload, "severity"),
			Description:          getStringFromPayload(payload, "description"),
			NamespacesOnThisNode: getStringFromPayload(payload, "namespacesonthisnode"),
		}

		// Numeric values
		if value, ok := payload["value"].(float64); ok {
			anomaly.Value = value
		}
		if threshold, ok := payload["threshold"].(float64); ok {
			anomaly.Threshold = threshold
		}

		// Timestamp (stored as unix seconds)
		if ts, ok := payload["timestamp"].(float64); ok {
			anomaly.Timestamp = time.Unix(int64(ts), 0)
		}

		// Labels map[string]string
		if labelsRaw, ok := payload["labels"].(map[string]interface{}); ok {
			labels := make(map[string]string, len(labelsRaw))
			for k, v := range labelsRaw {
				if sv, ok := v.(string); ok {
					labels[k] = sv
				} else {
					labels[k] = fmt.Sprintf("%v", v)
				}
			}
			anomaly.Labels = labels
		}

		// Metadata passthrough if object
		if md, ok := payload["metadata"].(map[string]interface{}); ok {
			anomaly.Metadata = md
		}

		// Events []types.Event (best-effort conversion)
		if evRaw, ok := payload["events"].([]interface{}); ok {
			events := make([]types.Event, 0, len(evRaw))
			for _, e := range evRaw {
				if m, ok := e.(map[string]interface{}); ok {
					evt := types.Event{}
					if s, ok := m["Type"].(string); ok {
						evt.Type = s
					}
					if s, ok := m["Reason"].(string); ok {
						evt.Reason = s
					}
					if s, ok := m["Message"].(string); ok {
						evt.Message = s
					}
					if num, ok := m["Timestamp"].(float64); ok {
						evt.Timestamp = time.Unix(int64(num), 0)
					}
					events = append(events, evt)
				}
			}
			if len(events) > 0 {
				anomaly.Events = events
			}
		}

		anomalies[i] = anomaly
	}

	return anomalies, nil
}

// getStringFromPayload safely extracts a string value from the payload
func getStringFromPayload(payload map[string]interface{}, key string) string {
	if value, ok := payload[key].(string); ok {
		return value
	}
	return ""
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
