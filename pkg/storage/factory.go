package storage

import (
	"fmt"
	"os"
	"strconv"
)

// StorageType represents the type of storage backend
type StorageType string

const (
	// StorageTypeQdrant represents Qdrant storage
	StorageTypeQdrant StorageType = "qdrant"
	// StorageTypeRedis represents Redis storage
	StorageTypeRedis StorageType = "redis"
)

// StorageConfig holds configuration for storage backends
type StorageConfig struct {
	Type     StorageType
	URL      string
	Password string
	DB       int
	// Qdrant-specific settings
	Collection string
	VectorSize int
	Distance   string
}

// NewStorage creates a new storage instance based on the configuration
func NewStorage(config StorageConfig) (Storage, error) {
	switch config.Type {
	case StorageTypeQdrant:
		return NewQdrantClient(config.URL, config.Collection, config.VectorSize, config.Distance)
	case StorageTypeRedis:
		return NewRedisClient(config.URL, config.Password, config.DB)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", config.Type)
	}
}

// NewStorageFromEnv creates a new storage instance from environment variables
func NewStorageFromEnv() (Storage, error) {
	storageType := StorageType(os.Getenv("STORAGE_TYPE"))
	if storageType == "" {
		storageType = StorageTypeQdrant // Default to Qdrant
	}

	config := StorageConfig{
		Type: storageType,
	}

	switch storageType {
	case StorageTypeQdrant:
		config.URL = os.Getenv("QDRANT_URL")
		if config.URL == "" {
			config.URL = "http://localhost:6333"
		}
		config.Collection = os.Getenv("QDRANT_COLLECTION")
		if config.Collection == "" {
			config.Collection = "alerts"
		}
		if v := os.Getenv("QDRANT_VECTOR_SIZE"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				config.VectorSize = n
			}
		}
		config.Distance = os.Getenv("QDRANT_DISTANCE")
		return NewStorage(config)

	case StorageTypeRedis:
		config.URL = os.Getenv("REDIS_URL")
		if config.URL == "" {
			config.URL = "localhost:6379"
		}
		config.Password = os.Getenv("REDIS_PASSWORD")
		db := os.Getenv("REDIS_DB")
		if db != "" {
			var err error
			config.DB, err = strconv.Atoi(db)
			if err != nil {
				return nil, fmt.Errorf("invalid REDIS_DB value: %v", err)
			}
		}
		return NewStorage(config)

	default:
		return nil, fmt.Errorf("unsupported storage type: %s", storageType)
	}
}
