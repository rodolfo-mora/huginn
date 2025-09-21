package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// Config represents the application configuration
type Config struct {
	Clusters            []ClusterConfig        `yaml:"clusters"`
	AnomalyDetection    AnomalyDetectionConfig `yaml:"anomalyDetection"`
	Storage             StorageConfig          `yaml:"storage"`
	Embedding           EmbeddingConfig        `yaml:"embedding"`
	Notification        NotificationConfig     `yaml:"notification"`
	Formatting          FormattingConfig       `yaml:"formatting"`
	ObservationInterval int                    `yaml:"observationInterval"` // Interval in seconds
}

// ClusterConfig represents configuration for a single Kubernetes cluster
type ClusterConfig struct {
	Name       string            `yaml:"name"`
	ID         string            `yaml:"id"`
	Labels     map[string]string `yaml:"labels"`
	Kubeconfig string            `yaml:"kubeconfig"`
	Context    string            `yaml:"context"`
	Namespace  string            `yaml:"namespace"`
	Resources  []string          `yaml:"resources"`
	Enabled    bool              `yaml:"enabled"`
}

// AnomalyDetectionConfig represents anomaly detection configuration
type AnomalyDetectionConfig struct {
	CPUThreshold        float64 `yaml:"cpuThreshold"`
	MemoryThreshold     float64 `yaml:"memoryThreshold"`
	PodRestartThreshold int     `yaml:"podRestartThreshold"`
	MaxHistorySize      int     `yaml:"maxHistorySize"`
	CPUAlpha            float64 `yaml:"cpuAlpha"`
	MemoryAlpha         float64 `yaml:"memoryAlpha"`
	RestartAlpha        float64 `yaml:"restartAlpha"`
	MinStdDev           float64 `yaml:"minStdDev"`
}

// StorageConfig represents storage configuration
type StorageConfig struct {
	Type        string       `yaml:"type"`
	StoreAlerts bool         `yaml:"storeAlerts"`
	Qdrant      QdrantConfig `yaml:"qdrant"`
	Redis       RedisConfig  `yaml:"redis"`
}

// QdrantConfig represents Qdrant-specific configuration
type QdrantConfig struct {
	URL            string `yaml:"url"`
	Collection     string `yaml:"collection"`
	VectorSize     int    `yaml:"vectorSize"`
	DistanceMetric string `yaml:"distanceMetric"`
}

// RedisConfig represents Redis-specific configuration
type RedisConfig struct {
	URL       string `yaml:"url"`
	Password  string `yaml:"password"`
	DB        int    `yaml:"db"`
	KeyPrefix string `yaml:"keyPrefix"`
}

// EmbeddingConfig represents embedding model configuration
type EmbeddingConfig struct {
	Type                 string                     `yaml:"type"`
	Dimension            int                        `yaml:"dimension"`
	OpenAI               OpenAIConfig               `yaml:"openai"`
	SentenceTransformers SentenceTransformersConfig `yaml:"sentenceTransformers"`
	Ollama               OllamaConfig               `yaml:"ollama"`
}

// OpenAIConfig represents OpenAI-specific configuration
type OpenAIConfig struct {
	APIKey string `yaml:"apiKey"`
	Model  string `yaml:"model"`
}

// SentenceTransformersConfig represents Sentence Transformers configuration
type SentenceTransformersConfig struct {
	Model  string `yaml:"model"`
	Device string `yaml:"device"`
}

// OllamaConfig represents Ollama-specific configuration
type OllamaConfig struct {
	URL   string `yaml:"url"`
	Model string `yaml:"model"`
}

// NotificationConfig represents notification configuration
type NotificationConfig struct {
	Enabled      bool               `yaml:"enabled"`
	Type         string             `yaml:"type"`
	MinSeverity  string             `yaml:"minSeverity"`
	Slack        SlackConfig        `yaml:"slack"`
	Email        EmailConfig        `yaml:"email"`
	Webhook      WebhookConfig      `yaml:"webhook"`
	Alertmanager AlertmanagerConfig `yaml:"alertmanager"`
}

// SlackConfig represents Slack-specific configuration
type SlackConfig struct {
	WebhookURL string `yaml:"webhookUrl"`
	Channel    string `yaml:"channel"`
	Username   string `yaml:"username"`
}

// EmailConfig represents email-specific configuration
type EmailConfig struct {
	SMTPHost     string   `yaml:"smtpHost"`
	SMTPPort     int      `yaml:"smtpPort"`
	SMTPUser     string   `yaml:"smtpUser"`
	SMTPPassword string   `yaml:"smtpPassword"`
	From         string   `yaml:"from"`
	To           []string `yaml:"to"`
}

// WebhookConfig represents webhook-specific configuration
type WebhookConfig struct {
	URL     string            `yaml:"url"`
	Method  string            `yaml:"method"`
	Headers map[string]string `yaml:"headers"`
}

// AlertmanagerConfig represents Alertmanager-specific configuration
type AlertmanagerConfig struct {
	URL           string            `yaml:"url"`
	DefaultLabels map[string]string `yaml:"defaultLabels"`
}

// FormattingConfig represents template-based formatting configuration
type FormattingConfig struct {
	AnomalyDisplayTemplate  string `yaml:"anomalyDisplayTemplate"`
	AnomalyEncodingTemplate string `yaml:"anomalyEncodingTemplate"`
}

// LoadConfig loads the configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	// Set defaults
	setDefaults(&config)

	return &config, nil
}

// setDefaults sets default values for configuration fields
func setDefaults(config *Config) {
	// Handle backward compatibility - if no clusters defined, create default cluster
	if len(config.Clusters) == 0 {
		// Try to read from old kubernetes config if it exists
		// This is for backward compatibility
		config.Clusters = []ClusterConfig{
			{
				Name:       "default",
				ID:         "default",
				Kubeconfig: filepath.Join(os.Getenv("HOME"), ".kube", "config"),
				Context:    "",
				Namespace:  "",
				Resources:  []string{"nodes", "events"},
				Enabled:    true,
				Labels:     make(map[string]string),
			},
		}
	}

	// Set defaults for each cluster
	for i := range config.Clusters {
		cluster := &config.Clusters[i]

		// Set cluster defaults
		if cluster.Kubeconfig == "" {
			cluster.Kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		}
		if cluster.Name == "" {
			cluster.Name = fmt.Sprintf("cluster-%d", i)
		}
		if cluster.ID == "" {
			cluster.ID = cluster.Name
		}
		if cluster.Labels == nil {
			cluster.Labels = make(map[string]string)
		}
		if len(cluster.Resources) == 0 {
			cluster.Resources = []string{"nodes", "events"}
		}
		if !cluster.Enabled {
			cluster.Enabled = true
		}
	}

	// Storage defaults
	if config.Storage.Type == "" {
		config.Storage.Type = "qdrant"
	}

	// Qdrant defaults
	if config.Storage.Qdrant.VectorSize == 0 {
		config.Storage.Qdrant.VectorSize = 384
	}
	if config.Storage.Qdrant.DistanceMetric == "" {
		config.Storage.Qdrant.DistanceMetric = "cosine"
	}

	// Redis defaults
	if config.Storage.Redis.KeyPrefix == "" {
		config.Storage.Redis.KeyPrefix = "huginn:"
	}

	// Anomaly detection defaults
	if config.AnomalyDetection.CPUThreshold == 0 {
		config.AnomalyDetection.CPUThreshold = 80.0
	}
	if config.AnomalyDetection.MemoryThreshold == 0 {
		config.AnomalyDetection.MemoryThreshold = 80.0
	}
	if config.AnomalyDetection.PodRestartThreshold == 0 {
		config.AnomalyDetection.PodRestartThreshold = 3
	}
	if config.AnomalyDetection.MaxHistorySize == 0 {
		config.AnomalyDetection.MaxHistorySize = 1000
	}

	// Alpha defaults for EWMA smoothing
	if config.AnomalyDetection.CPUAlpha == 0 {
		config.AnomalyDetection.CPUAlpha = 0.3
	}
	if config.AnomalyDetection.MemoryAlpha == 0 {
		config.AnomalyDetection.MemoryAlpha = 0.3
	}
	if config.AnomalyDetection.RestartAlpha == 0 {
		config.AnomalyDetection.RestartAlpha = 0.3
	}

	// Minimum standard deviation default
	if config.AnomalyDetection.MinStdDev == 0 {
		config.AnomalyDetection.MinStdDev = 1.0
	}

	// Embedding defaults
	if config.Embedding.Type == "" {
		config.Embedding.Type = "simple"
	}
	if config.Embedding.Dimension == 0 {
		config.Embedding.Dimension = 384
	}

	// Ollama defaults
	if config.Embedding.Ollama.URL == "" {
		config.Embedding.Ollama.URL = "http://localhost:11434"
	}
	if config.Embedding.Ollama.Model == "" {
		config.Embedding.Ollama.Model = "nomic-embed-text"
	}

	// OpenAI defaults
	if config.Embedding.OpenAI.Model == "" {
		config.Embedding.OpenAI.Model = "text-embedding-ada-002"
	}

	// Sentence Transformers defaults
	if config.Embedding.SentenceTransformers.Model == "" {
		config.Embedding.SentenceTransformers.Model = "all-MiniLM-L6-v2"
	}
	if config.Embedding.SentenceTransformers.Device == "" {
		config.Embedding.SentenceTransformers.Device = "cpu"
	}

	// Notification defaults
	if config.Notification.MinSeverity == "" {
		config.Notification.MinSeverity = "warning"
	}

	// Observation interval default
	if config.ObservationInterval == 0 {
		config.ObservationInterval = 30 // Default to 30 seconds
	}

	// Formatting defaults
	if config.Formatting.AnomalyDisplayTemplate == "" {
		config.Formatting.AnomalyDisplayTemplate = "Cluster {{.ClusterName}} [{{.Severity}}] {{.Type}} in {{.ResourceType}} resource {{.Resource}} in namespace {{.Namespace}}: {{.Description}}\n"
	}
	if config.Formatting.AnomalyEncodingTemplate == "" {
		config.Formatting.AnomalyEncodingTemplate = "Anomaly detected of type {{.Type}} in {{.ResourceType}} resource {{.Resource}} in namespace {{.Namespace}} in cluster {{.ClusterName}}: {{.Description}}"
	}
}
