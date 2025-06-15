package agent

import (
	"fmt"
	"log"
	"time"

	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/rodgon/valkyrie/pkg/anomaly"
	"github.com/rodgon/valkyrie/pkg/config"
	"github.com/rodgon/valkyrie/pkg/embedding"
	"github.com/rodgon/valkyrie/pkg/notification"
	"github.com/rodgon/valkyrie/pkg/storage"
	"github.com/rodgon/valkyrie/pkg/types"
)

// Agent represents the main agent that observes and learns from the cluster
type Agent struct {
	k8sClient    *kubernetes.Clientset
	state        types.ClusterState
	observations []types.Observation
	detector     *anomaly.Detector
	notifier     notification.Notifier
	storage      storage.Storage
	model        embedding.Model
	config       *config.Config
}

// NewAgent creates a new agent instance
func NewAgent(cfg *config.Config) (*Agent, error) {
	// Load kubeconfig
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %v", err)
	}

	// Create Kubernetes client
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %v", err)
	}

	// Create detector
	detector := anomaly.NewDetector()
	detector.SetThresholds(
		cfg.AnomalyDetection.CPUThreshold,
		cfg.AnomalyDetection.MemoryThreshold,
		cfg.AnomalyDetection.PodRestartThreshold,
	)
	detector.SetMaxHistorySize(cfg.AnomalyDetection.MaxHistorySize)

	// Create storage client
	storageConfig := storage.StorageConfig{
		Type:     storage.StorageType(cfg.Storage.Type),
		URL:      cfg.Storage.Qdrant.URL,
		Password: cfg.Storage.Redis.Password,
		DB:       cfg.Storage.Redis.DB,
	}
	storageClient, err := storage.NewStorage(storageConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage client: %v", err)
	}

	// Create embedding model
	var model embedding.Model
	switch cfg.Embedding.Type {
	case "simple":
		model = embedding.NewSimpleModel(cfg.Embedding.Dimension)
	case "openai":
		model = embedding.NewOpenAIModel(cfg.Embedding.OpenAI.APIKey, cfg.Embedding.OpenAI.Model, cfg.Embedding.Dimension)
	case "sentence-transformers":
		model = embedding.NewSentenceTransformersModel(cfg.Embedding.SentenceTransformers.Model, cfg.Embedding.SentenceTransformers.Device, cfg.Embedding.Dimension)
	default:
		return nil, fmt.Errorf("unsupported embedding type: %s", cfg.Embedding.Type)
	}

	// Create notifier
	var notifier notification.Notifier
	switch cfg.Notification.Type {
	case "slack":
		notifier = &notification.SlackNotifier{WebhookURL: cfg.Notification.Slack.WebhookURL}
	case "email":
		notifier = &notification.EmailNotifier{
			SMTPHost:     cfg.Notification.Email.SMTPHost,
			SMTPPort:     cfg.Notification.Email.SMTPPort,
			SMTPUser:     cfg.Notification.Email.SMTPUser,
			SMTPPassword: cfg.Notification.Email.SMTPPassword,
			From:         cfg.Notification.Email.From,
			To:           cfg.Notification.Email.To,
		}
	case "webhook":
		notifier = &notification.WebhookNotifier{
			URL:     cfg.Notification.Webhook.URL,
			Headers: cfg.Notification.Webhook.Headers,
		}
	case "alertmanager":
		notifier = &notification.AlertmanagerNotifier{
			URL:           cfg.Notification.Alertmanager.URL,
			DefaultLabels: cfg.Notification.Alertmanager.DefaultLabels,
		}
	default:
		return nil, fmt.Errorf("unsupported notification type: %s", cfg.Notification.Type)
	}

	return &Agent{
		k8sClient:    clientset,
		detector:     detector,
		notifier:     notifier,
		storage:      storageClient,
		model:        model,
		config:       cfg,
		observations: make([]types.Observation, 0),
	}, nil
}

// ObserveCluster collects the current state of the cluster
func (a *Agent) ObserveCluster() error {
	// TODO: Implement cluster state collection
	// This is a placeholder that creates a dummy state
	a.state = types.ClusterState{
		Namespaces: []string{"default"},
		Nodes: []types.Node{
			{
				Name:        "node-1",
				CPUUsage:    "50",
				MemoryUsage: "60",
				Status:      "Ready",
			},
		},
		Resources: map[string]types.ResourceList{
			"default": {
				Pods: []types.Pod{
					{
						Name:         "pod-1",
						Status:       "Running",
						RestartCount: 0,
					},
				},
			},
		},
	}
	return nil
}

// Learn processes the current state and updates the agent's knowledge
func (a *Agent) Learn() error {
	// Calculate reward based on cluster health
	reward := 1.0
	for _, node := range a.state.Nodes {
		if node.Status != "Ready" {
			reward -= 0.5
		}
	}

	// Store observation
	observation := types.Observation{
		Timestamp: time.Now(),
		State:     a.state,
		Reward:    reward,
	}
	a.observations = append(a.observations, observation)

	// Keep history size limited
	if len(a.observations) > a.config.AnomalyDetection.MaxHistorySize {
		a.observations = a.observations[len(a.observations)-a.config.AnomalyDetection.MaxHistorySize:]
	}

	return nil
}

// DetectAnomalies checks for anomalies in the current state
func (a *Agent) DetectAnomalies() ([]types.Anomaly, error) {
	anomalies := a.detector.DetectAnomalies(a.state)

	// Store anomalies in vector database
	for _, anomaly := range anomalies {
		// Generate embedding for the anomaly
		text := fmt.Sprintf("%s %s %s %s", anomaly.Type, anomaly.Resource, anomaly.Namespace, anomaly.Description)
		vector, err := a.model.Encode(text)
		if err != nil {
			log.Printf("Failed to generate embedding for anomaly: %v", err)
			continue
		}

		// Store in vector database
		err = a.storage.StoreAlert(vector, anomaly)
		if err != nil {
			log.Printf("Failed to store anomaly in vector database: %v", err)
			continue
		}

		// Send notification if severity is high enough
		if shouldNotify(anomaly, a.config.Notification.MinSeverity) {
			err = a.notifier.Notify(anomaly)
			if err != nil {
				log.Printf("Failed to send notification for anomaly: %v", err)
			}
		}
	}

	return anomalies, nil
}

// shouldNotify determines if a notification should be sent based on severity
func shouldNotify(anomaly types.Anomaly, minSeverity string) bool {
	severityLevels := map[string]int{
		"low":    1,
		"medium": 2,
		"high":   3,
	}

	return severityLevels[anomaly.Severity] >= severityLevels[minSeverity]
}

// PrintState prints the current state of the cluster
func (a *Agent) PrintState() {
	fmt.Printf("Current cluster state:\n")
	fmt.Printf("Namespaces: %v\n", a.state.Namespaces)
	fmt.Printf("Nodes: %d\n", len(a.state.Nodes))
	for _, node := range a.state.Nodes {
		fmt.Printf("  - %s: CPU=%s, Memory=%s, Status=%s\n",
			node.Name, node.CPUUsage, node.MemoryUsage, node.Status)
	}
}

// PrintAnomalies prints the detected anomalies
func (a *Agent) PrintAnomalies(anomalies []types.Anomaly) {
	if len(anomalies) == 0 {
		fmt.Println("No anomalies detected")
		return
	}

	fmt.Printf("Detected %d anomalies:\n", len(anomalies))
	for _, anomaly := range anomalies {
		fmt.Printf("  - [%s] %s (%s): %s\n",
			anomaly.Severity, anomaly.Type, anomaly.Resource, anomaly.Description)
	}
}
