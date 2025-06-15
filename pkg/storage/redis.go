package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rodgon/valkyrie/pkg/types"
)

// RedisClient implements the Storage interface using Redis
type RedisClient struct {
	client *redis.Client
	ctx    context.Context
}

// NewRedisClient creates a new Redis client
func NewRedisClient(addr, password string, db int) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("error connecting to Redis: %v", err)
	}

	return &RedisClient{
		client: client,
		ctx:    ctx,
	}, nil
}

// StoreAlert implements the Storage interface
func (r *RedisClient) StoreAlert(anomaly types.Anomaly, events []types.Event, vector []float32) error {
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

	// Marshal alert to JSON
	data, err := json.Marshal(alertVector)
	if err != nil {
		return fmt.Errorf("error marshaling alert: %v", err)
	}

	// Store in Redis
	key := fmt.Sprintf("alert:%s", alertVector.ID)
	if err := r.client.Set(r.ctx, key, data, 0).Err(); err != nil {
		return fmt.Errorf("error storing alert in Redis: %v", err)
	}

	// Add to indexes
	if err := r.client.SAdd(r.ctx, "alerts:all", alertVector.ID).Err(); err != nil {
		return fmt.Errorf("error adding to all alerts index: %v", err)
	}

	if alertVector.Payload.Namespace != "" {
		if err := r.client.SAdd(r.ctx, fmt.Sprintf("alerts:namespace:%s", alertVector.Payload.Namespace), alertVector.ID).Err(); err != nil {
			return fmt.Errorf("error adding to namespace index: %v", err)
		}
	}

	if alertVector.Payload.Severity != "" {
		if err := r.client.SAdd(r.ctx, fmt.Sprintf("alerts:severity:%s", alertVector.Payload.Severity), alertVector.ID).Err(); err != nil {
			return fmt.Errorf("error adding to severity index: %v", err)
		}
	}

	// Add to time-based index
	timeKey := fmt.Sprintf("alerts:time:%d", alertVector.Timestamp.Unix())
	if err := r.client.SAdd(r.ctx, timeKey, alertVector.ID).Err(); err != nil {
		return fmt.Errorf("error adding to time index: %v", err)
	}

	return nil
}

// SearchSimilarAlerts implements the Storage interface
func (r *RedisClient) SearchSimilarAlerts(vector []float32, limit int) ([]AlertVector, error) {
	// Note: Redis doesn't have built-in vector similarity search
	// In a production environment, you would use RedisAI or RedisSearch with vector similarity
	// For now, we'll return an error
	return nil, fmt.Errorf("vector similarity search not implemented for Redis")
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
