package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rodolfo-mora/huginn/pkg/types"
)

// RedisClient implements the Storage interface using Redis
type RedisClient struct {
	client *redis.Client
	ctx    context.Context
}

// NewRedisClient creates a new Redis client
func NewRedisClient(url, password string, db int) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     url,
		Password: password,
		DB:       db,
	})

	// Test connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %v", err)
	}

	return &RedisClient{
		client: client,
		ctx:    ctx,
	}, nil
}

// StoreAlert stores an alert in Redis
func (c *RedisClient) StoreAlert(vector []float32, anomaly types.Anomaly) error {
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

	// Store in Redis
	key := fmt.Sprintf("alert:%s", alertVector.ID)
	if err := c.client.Set(c.ctx, key, data, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to store alert in Redis: %v", err)
	}

	// Add to vector index
	// Note: This is a simplified implementation. In a real system, you would use
	// a proper vector similarity search library or Redis module.
	vectorKey := fmt.Sprintf("vector:%s", alertVector.ID)
	if err := c.client.Set(c.ctx, vectorKey, vector, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to store vector in Redis: %v", err)
	}

	return nil
}

// SearchSimilarAlerts searches for similar alerts in Redis
func (c *RedisClient) SearchSimilarAlerts(vector []float32, limit int) ([]types.Anomaly, error) {
	// Note: This is a simplified implementation. In a real system, you would use
	// a proper vector similarity search library or Redis module.
	// This implementation just returns the most recent alerts.

	// Get all vector keys
	keys, err := c.client.Keys(c.ctx, "vector:*").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get vector keys: %v", err)
	}

	// Get the most recent alerts
	var anomalies []types.Anomaly
	for _, key := range keys {
		// Get alert data
		alertKey := fmt.Sprintf("alert:%s", key[7:]) // Remove "vector:" prefix
		data, err := c.client.Get(c.ctx, alertKey).Bytes()
		if err != nil {
			continue
		}

		// Unmarshal alert
		var alertVector AlertVector
		if err := json.Unmarshal(data, &alertVector); err != nil {
			continue
		}

		// Convert to anomaly
		anomaly := types.Anomaly{
			Type:        alertVector.Payload.Type,
			Resource:    alertVector.Payload.Resource,
			Namespace:   alertVector.Payload.Namespace,
			Severity:    alertVector.Payload.Severity,
			Description: alertVector.Payload.Description,
			Value:       alertVector.Payload.Value,
			Threshold:   alertVector.Payload.Threshold,
			Labels:      alertVector.Payload.Labels,
			Events:      alertVector.Payload.Events,
			Metadata:    alertVector.Payload.Metadata,
		}

		anomalies = append(anomalies, anomaly)
		if len(anomalies) >= limit {
			break
		}
	}

	return anomalies, nil
}

// GetAlert implements the Storage interface
func (r *RedisClient) GetAlert(id string) (*AlertVector, error) {
	key := fmt.Sprintf("alert:%s", id)
	data, err := r.client.Get(r.ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("alert not found: %s", id)
		}
		return nil, fmt.Errorf("error getting alert from Redis: %v", err)
	}

	var alert AlertVector
	if err := json.Unmarshal(data, &alert); err != nil {
		return nil, fmt.Errorf("error unmarshaling alert: %v", err)
	}

	return &alert, nil
}

// ListAlerts implements the Storage interface
func (r *RedisClient) ListAlerts(namespace, severity string, startTime, endTime time.Time) ([]AlertVector, error) {
	var alertIDs []string
	var err error

	// Get base set of alerts
	if namespace != "" && severity != "" {
		// Intersect namespace and severity sets
		key := fmt.Sprintf("alerts:namespace:%s", namespace)
		severityKey := fmt.Sprintf("alerts:severity:%s", severity)
		alertIDs, err = r.client.SInter(r.ctx, key, severityKey).Result()
	} else if namespace != "" {
		alertIDs, err = r.client.SMembers(r.ctx, fmt.Sprintf("alerts:namespace:%s", namespace)).Result()
	} else if severity != "" {
		alertIDs, err = r.client.SMembers(r.ctx, fmt.Sprintf("alerts:severity:%s", severity)).Result()
	} else {
		alertIDs, err = r.client.SMembers(r.ctx, "alerts:all").Result()
	}

	if err != nil {
		return nil, fmt.Errorf("error getting alert IDs: %v", err)
	}

	// Filter by time range
	var filteredIDs []string
	for _, id := range alertIDs {
		alert, err := r.GetAlert(id)
		if err != nil {
			continue
		}
		if alert.Timestamp.After(startTime) && alert.Timestamp.Before(endTime) {
			filteredIDs = append(filteredIDs, id)
		}
	}

	// Get full alert data
	var alerts []AlertVector
	for _, id := range filteredIDs {
		alert, err := r.GetAlert(id)
		if err != nil {
			continue
		}
		alerts = append(alerts, *alert)
	}

	return alerts, nil
}

// DeleteAlert implements the Storage interface
func (r *RedisClient) DeleteAlert(id string) error {
	alert, err := r.GetAlert(id)
	if err != nil {
		return err
	}

	// Delete from main storage
	key := fmt.Sprintf("alert:%s", id)
	if err := r.client.Del(r.ctx, key).Err(); err != nil {
		return fmt.Errorf("error deleting alert from Redis: %v", err)
	}

	// Remove from indexes
	if err := r.client.SRem(r.ctx, "alerts:all", id).Err(); err != nil {
		return fmt.Errorf("error removing from all alerts index: %v", err)
	}

	if alert.Payload.Namespace != "" {
		if err := r.client.SRem(r.ctx, fmt.Sprintf("alerts:namespace:%s", alert.Payload.Namespace), id).Err(); err != nil {
			return fmt.Errorf("error removing from namespace index: %v", err)
		}
	}

	if alert.Payload.Severity != "" {
		if err := r.client.SRem(r.ctx, fmt.Sprintf("alerts:severity:%s", alert.Payload.Severity), id).Err(); err != nil {
			return fmt.Errorf("error removing from severity index: %v", err)
		}
	}

	timeKey := fmt.Sprintf("alerts:time:%d", alert.Timestamp.Unix())
	if err := r.client.SRem(r.ctx, timeKey, id).Err(); err != nil {
		return fmt.Errorf("error removing from time index: %v", err)
	}

	return nil
}
