package agent

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rodgon/valkyrie/pkg/anomaly"
	"github.com/rodgon/valkyrie/pkg/cluster"
	"github.com/rodgon/valkyrie/pkg/config"
	"github.com/rodgon/valkyrie/pkg/embedding"
	"github.com/rodgon/valkyrie/pkg/metrics"
	"github.com/rodgon/valkyrie/pkg/notification"
	"github.com/rodgon/valkyrie/pkg/storage"
	"github.com/rodgon/valkyrie/pkg/types"
)

// MultiClusterAgent orchestrates multiple cluster agents
type MultiClusterAgent struct {
	config         *config.Config
	clusterManager *cluster.Manager
	agents         map[string]*Agent
	detector       *anomaly.Detector
	notifier       notification.Notifier
	storage        storage.Storage
	model          embedding.Model
	metrics        *metrics.PrometheusExporter
	metricsServer  *metrics.MetricsServer
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewMultiClusterAgent creates a new multi-cluster agent
func NewMultiClusterAgent(cfg *config.Config) (*MultiClusterAgent, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create cluster manager
	clusterManager := cluster.NewManager(cfg)
	if err := clusterManager.InitializeClusters(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize clusters: %v", err)
	}

	// Create detector
	detector := anomaly.NewDetector(
		cfg.AnomalyDetection.CPUThreshold,
		cfg.AnomalyDetection.MemoryThreshold,
		cfg.AnomalyDetection.PodRestartThreshold,
		cfg.AnomalyDetection.MaxHistorySize,
		cfg.AnomalyDetection.CPUAlpha,
		cfg.AnomalyDetection.MemoryAlpha,
		cfg.AnomalyDetection.RestartAlpha,
		false, // debug mode
		cfg.AnomalyDetection.MinStdDev,
	)

	// Create Prometheus metrics exporter
	metricsExporter := metrics.NewPrometheusExporter(detector, cfg)

	// Create metrics server
	metricsServer := metrics.NewMetricsServer(":8080", metricsExporter)

	// Create storage client
	var storageClient storage.Storage
	if cfg.Storage.StoreAlerts {
		storageConfig := storage.StorageConfig{
			Type:     storage.StorageType(cfg.Storage.Type),
			URL:      cfg.Storage.Qdrant.URL,
			Password: cfg.Storage.Redis.Password,
			DB:       cfg.Storage.Redis.DB,
		}

		var err error
		storageClient, err = storage.NewStorage(storageConfig)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create storage client: %v", err)
		}
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
	case "ollama":
		model = embedding.NewOllamaModel(cfg.Embedding.Ollama.URL, cfg.Embedding.Ollama.Model, cfg.Embedding.Dimension)
	default:
		cancel()
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
		cancel()
		return nil, fmt.Errorf("unsupported notification type: %s", cfg.Notification.Type)
	}

	multiAgent := &MultiClusterAgent{
		config:         cfg,
		clusterManager: clusterManager,
		agents:         make(map[string]*Agent),
		detector:       detector,
		notifier:       notifier,
		storage:        storageClient,
		model:          model,
		metrics:        metricsExporter,
		metricsServer:  metricsServer,
		ctx:            ctx,
		cancel:         cancel,
	}

	// Create individual cluster agents
	if err := multiAgent.createClusterAgents(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create cluster agents: %v", err)
	}

	return multiAgent, nil
}

// createClusterAgents creates individual agents for each cluster
func (m *MultiClusterAgent) createClusterAgents() error {
	for _, clusterConfig := range m.config.Clusters {
		if !clusterConfig.Enabled {
			continue
		}

		// Create a single-cluster config for this cluster
		singleClusterConfig := &config.Config{
			Clusters:         []config.ClusterConfig{clusterConfig},
			AnomalyDetection: m.config.AnomalyDetection,
			Storage:          m.config.Storage,
			Embedding:        m.config.Embedding,
			Notification:     m.config.Notification,
		}

		// Create agent without metrics (we'll use the shared metrics from multi-agent)
		agent, err := NewAgentWithoutMetrics(singleClusterConfig)
		if err != nil {
			log.Printf("Warning: failed to create agent for cluster %s: %v", clusterConfig.Name, err)
			m.clusterManager.SetClusterHealth(clusterConfig.ID, false, err)
			continue
		}

		// Set the shared metrics and storage
		agent.metrics = m.metrics
		agent.storage = m.storage
		agent.notifier = m.notifier
		agent.model = m.model

		m.agents[clusterConfig.ID] = agent
		log.Printf("Created agent for cluster: %s (%s)", clusterConfig.Name, clusterConfig.ID)
	}

	return nil
}

// ObserveAllClusters observes all enabled clusters
func (m *MultiClusterAgent) ObserveAllClusters() error {
	var wg sync.WaitGroup
	errors := make(chan error, len(m.agents))

	for clusterID, agent := range m.agents {
		wg.Add(1)
		go func(id string, a *Agent) {
			defer wg.Done()

			if err := a.ObserveCluster(); err != nil {
				log.Printf("Error observing cluster %s: %v", id, err)
				m.clusterManager.SetClusterHealth(id, false, err)
				errors <- fmt.Errorf("cluster %s: %v", id, err)
				return
			}

			// Update cluster state
			state := a.state
			state.ClusterID = id
			if cluster, exists := m.clusterManager.GetCluster(id); exists {
				state.ClusterName = cluster.ClusterConfig.Name
			}

			if err := m.clusterManager.UpdateClusterState(id, &state); err != nil {
				log.Printf("Error updating cluster state for %s: %v", id, err)
			}
		}(clusterID, agent)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	hasErrors := false
	for err := range errors {
		hasErrors = true
		log.Printf("Cluster observation error: %v", err)
	}

	if hasErrors {
		return fmt.Errorf("some clusters failed to observe")
	}

	return nil
}

// DetectAllAnomalies detects anomalies across all clusters
func (m *MultiClusterAgent) DetectAllAnomalies() ([]types.Anomaly, error) {
	var allAnomalies []types.Anomaly
	var wg sync.WaitGroup
	anomalyChan := make(chan []types.Anomaly, len(m.agents))

	for clusterID, agent := range m.agents {
		wg.Add(1)
		go func(id string, a *Agent) {
			defer wg.Done()

			anomalies, err := a.DetectAnomalies()
			if err != nil {
				log.Printf("Error detecting anomalies for cluster %s: %v", id, err)
				return
			}

			// Add cluster context to anomalies
			for i := range anomalies {
				anomalies[i].ClusterID = id
				if cluster, exists := m.clusterManager.GetCluster(id); exists {
					anomalies[i].ClusterName = cluster.ClusterConfig.Name
				}
			}

			anomalyChan <- anomalies
		}(clusterID, agent)
	}

	wg.Wait()
	close(anomalyChan)

	// Collect all anomalies
	for anomalies := range anomalyChan {
		allAnomalies = append(allAnomalies, anomalies...)
	}

	return allAnomalies, nil
}

// LearnFromAllClusters learns from observations across all clusters
func (m *MultiClusterAgent) LearnFromAllClusters() error {
	for clusterID, agent := range m.agents {
		if err := agent.Learn(); err != nil {
			log.Printf("Error learning from cluster %s: %v", clusterID, err)
		}
	}
	return nil
}

// PrintMultiClusterState prints the state of all clusters
func (m *MultiClusterAgent) PrintMultiClusterState() {
	multiState := m.clusterManager.GetMultiClusterState()
	summary := multiState.Summary

	fmt.Printf("Multi-Cluster State Summary:\n")
	fmt.Printf("Total Clusters: %d\n", summary.TotalClusters)
	fmt.Printf("Healthy Clusters: %d\n", summary.HealthyClusters)
	fmt.Printf("Unhealthy Clusters: %d\n", summary.UnhealthyClusters)
	fmt.Printf("Total Nodes: %d\n", summary.TotalNodes)
	fmt.Printf("Last Updated: %s\n", summary.LastUpdated.Format(time.RFC3339))

	for clusterID, state := range multiState.Clusters {
		cluster, exists := m.clusterManager.GetCluster(clusterID)
		status := "Healthy"
		if exists && !cluster.Healthy {
			status = "Unhealthy"
		}

		fmt.Printf("\nCluster: %s (%s) - %s\n", state.ClusterName, clusterID, status)
		fmt.Printf("  Nodes: %d\n", len(state.Nodes))
		fmt.Printf("  Namespaces: %d\n", len(state.Namespaces))
		fmt.Printf("  Events: %d\n", len(state.Events))
	}
}

// PrintAllAnomalies prints anomalies from all clusters
func (m *MultiClusterAgent) PrintAllAnomalies(anomalies []types.Anomaly) {
	if len(anomalies) == 0 {
		fmt.Println("No anomalies detected across all clusters")
		return
	}

	fmt.Printf("Detected %d anomalies across all clusters:\n", len(anomalies))
	for _, anomaly := range anomalies {
		fmt.Printf("  - [%s] %s/%s (%s - %s): %s\n",
			anomaly.Severity, anomaly.ClusterName, anomaly.Resource, anomaly.Type, anomaly.Namespace, anomaly.Description)
	}
}

// StartMetricsServer starts the Prometheus metrics server
func (m *MultiClusterAgent) StartMetricsServer() {
	m.metricsServer.StartAsync()
}

// Stop stops the multi-cluster agent
func (m *MultiClusterAgent) Stop() {
	m.cancel()
	m.clusterManager.Stop()
}

// GetClusterManager returns the cluster manager
func (m *MultiClusterAgent) GetClusterManager() *cluster.Manager {
	return m.clusterManager
}

// PrintConfig prints the current configuration
func (m *MultiClusterAgent) PrintConfig() {
	fmt.Printf("Multi-Cluster Configuration:\n")
	fmt.Printf("Total Clusters: %d\n", len(m.config.Clusters))

	for i, cluster := range m.config.Clusters {
		status := "Disabled"
		if cluster.Enabled {
			status = "Enabled"
		}

		fmt.Printf("\nCluster %d: %s (%s) - %s\n", i+1, cluster.Name, cluster.ID, status)
		fmt.Printf("  Kubeconfig: %s\n", cluster.Kubeconfig)
		if cluster.Context != "" {
			fmt.Printf("  Context: %s\n", cluster.Context)
		}
		if cluster.Namespace != "" {
			fmt.Printf("  Namespace: %s\n", cluster.Namespace)
		} else {
			fmt.Printf("  Namespace: all\n")
		}
		fmt.Printf("  Resources: %v\n", cluster.Resources)
		if len(cluster.Labels) > 0 {
			fmt.Printf("  Labels: %v\n", cluster.Labels)
		}
	}

	fmt.Printf("\nAnomaly Detection:\n")
	fmt.Printf("  CPU Threshold: %.1f%%\n", m.config.AnomalyDetection.CPUThreshold)
	fmt.Printf("  Memory Threshold: %.1f%%\n", m.config.AnomalyDetection.MemoryThreshold)
	fmt.Printf("  Pod Restart Threshold: %d\n", m.config.AnomalyDetection.PodRestartThreshold)
	fmt.Printf("  Max History Size: %d\n", m.config.AnomalyDetection.MaxHistorySize)

	fmt.Printf("\nStorage:\n")
	fmt.Printf("  Type: %s\n", m.config.Storage.Type)
	fmt.Printf("  Store Alerts: %t\n", m.config.Storage.StoreAlerts)
	if m.config.Storage.Type == "qdrant" {
		fmt.Printf("  Qdrant URL: %s\n", m.config.Storage.Qdrant.URL)
		fmt.Printf("  Collection: %s\n", m.config.Storage.Qdrant.Collection)
	}

	fmt.Printf("\nEmbedding:\n")
	fmt.Printf("  Type: %s\n", m.config.Embedding.Type)
	fmt.Printf("  Dimension: %d\n", m.config.Embedding.Dimension)
	if m.config.Embedding.Type == "ollama" {
		fmt.Printf("  Ollama URL: %s\n", m.config.Embedding.Ollama.URL)
		fmt.Printf("  Model: %s\n", m.config.Embedding.Ollama.Model)
	}

	fmt.Printf("\nNotification:\n")
	fmt.Printf("  Enabled: %t\n", m.config.Notification.Enabled)
	if m.config.Notification.Enabled {
		fmt.Printf("  Type: %s\n", m.config.Notification.Type)
		fmt.Printf("  Min Severity: %s\n", m.config.Notification.MinSeverity)
	}
}
