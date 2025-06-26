package agent

import (
	"fmt"
	"log"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"context"

	"github.com/rodgon/valkyrie/pkg/anomaly"
	"github.com/rodgon/valkyrie/pkg/config"
	"github.com/rodgon/valkyrie/pkg/embedding"
	"github.com/rodgon/valkyrie/pkg/metrics"
	"github.com/rodgon/valkyrie/pkg/notification"
	"github.com/rodgon/valkyrie/pkg/storage"
	"github.com/rodgon/valkyrie/pkg/types"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsapi "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

// Agent represents the main agent that observes and learns from the cluster
type Agent struct {
	k8sClient     *kubernetes.Clientset
	config        *config.Config
	restConfig    *rest.Config
	state         types.ClusterState
	observations  []types.Observation
	detector      *anomaly.Detector
	notifier      notification.Notifier
	storage       storage.Storage
	model         embedding.Model
	metrics       *metrics.PrometheusExporter
	metricsServer *metrics.MetricsServer
}

// NewAgent creates a new agent instance
func NewAgent(cfg *config.Config) (*Agent, error) {
	// Load kubeconfig
	kubeconfig := cfg.Kubernetes.Kubeconfig
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
	detector := anomaly.NewDetector(
		cfg.AnomalyDetection.CPUThreshold,
		cfg.AnomalyDetection.MemoryThreshold,
		cfg.AnomalyDetection.PodRestartThreshold,
		cfg.AnomalyDetection.MaxHistorySize,
		cfg.AnomalyDetection.CPUAlpha,
		cfg.AnomalyDetection.MemoryAlpha,
		cfg.AnomalyDetection.RestartAlpha,
		false, // debug mode - set to true to enable detailed logging
		cfg.AnomalyDetection.MinStdDev,
	)

	// Create Prometheus metrics exporter
	metricsExporter := metrics.NewPrometheusExporter(detector, cfg)

	// Create metrics server
	metricsServer := metrics.NewMetricsServer(":8080", metricsExporter)

	var storageClient storage.Storage
	if cfg.Storage.StoreAlerts {
		// Create storage client
		storageConfig := storage.StorageConfig{
			Type:     storage.StorageType(cfg.Storage.Type),
			URL:      cfg.Storage.Qdrant.URL,
			Password: cfg.Storage.Redis.Password,
			DB:       cfg.Storage.Redis.DB,
		}

		storageClient, err = storage.NewStorage(storageConfig)
		if err != nil {
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
		k8sClient:     clientset,
		restConfig:    config,
		detector:      detector,
		notifier:      notifier,
		storage:       storageClient,
		model:         model,
		config:        cfg,
		observations:  make([]types.Observation, 0),
		metrics:       metricsExporter,
		metricsServer: metricsServer,
	}, nil
}

// ObserveCluster collects the current state of the cluster
func (a *Agent) ObserveCluster() error {
	ctx := context.Background()

	// Create metrics client
	metricsClient, err := metricsv.NewForConfig(a.restConfig)
	if err != nil {
		return fmt.Errorf("failed to create metrics client: %v", err)
	}

	// Initialize cluster state
	var nodes []types.Node
	var resources = make(map[string]types.ResourceList)

	// Collect node data if configured
	if a.shouldCollectResource("nodes") {
		nodes, err = a.collectNodes(ctx, metricsClient)
		if err != nil {
			return fmt.Errorf("failed to collect nodes: %v", err)
		}
	}

	// Collect namespaces (always needed for resource organization)
	// TODO: This is a hack to get the namespaces. We should use the API to get the namespaces.
	nsList, err := a.k8sClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %v", err)
	}

	// Collect resource data per namespace
	for _, ns := range nsList.Items {
		resourceList := types.ResourceList{}

		// Collect pods if configured
		if a.shouldCollectResource("pods") {
			pods, err := a.collectPods(ctx, ns.Name)
			if err != nil {
				log.Printf("Warning: failed to collect pods in namespace %s: %v", ns.Name, err)
			} else {
				resourceList.Pods = pods
			}
		}

		// Collect services if configured
		if a.shouldCollectResource("services") {
			services, err := a.collectServices(ctx, ns.Name)
			if err != nil {
				log.Printf("Warning: failed to collect services in namespace %s: %v", ns.Name, err)
			} else {
				resourceList.Services = services
			}
		}

		// Collect deployments if configured
		if a.shouldCollectResource("deployments") {
			deployments, err := a.collectDeployments(ctx, ns.Name)
			if err != nil {
				log.Printf("Warning: failed to collect deployments in namespace %s: %v", ns.Name, err)
			} else {
				resourceList.Deployments = deployments
			}
		}

		// Only add namespace to resources if we collected any data
		if len(resourceList.Pods) > 0 || len(resourceList.Services) > 0 || len(resourceList.Deployments) > 0 {
			resources[ns.Name] = resourceList
		}
	}

	// Build namespace names list
	nsNames := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		nsNames = append(nsNames, ns.Name)
	}

	a.state = types.ClusterState{
		Namespaces: nsNames,
		Nodes:      nodes,
		Resources:  resources,
	}
	return nil
}

// collectNodes collects node data including metrics
func (a *Agent) collectNodes(ctx context.Context, metricsClient *metricsv.Clientset) ([]types.Node, error) {
	nodeList, err := a.k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %v", err)
	}

	var nodeMetrics *metricsapi.NodeMetricsList
	if metricsClient != nil {
		nodeMetrics, err = metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Printf("Warning: failed to get node metrics: %v", err)
		}
	}

	nodes := make([]types.Node, 0, len(nodeList.Items))
	for _, node := range nodeList.Items {
		cpuUsage := "0"
		memoryUsage := "0"
		if nodeMetrics != nil {
			for _, m := range nodeMetrics.Items {
				if m.Name == node.Name {
					cpuUsage = m.Usage.Cpu().AsDec().String()
					memoryUsage = m.Usage.Memory().AsDec().String()
					break
				}
			}
		}

		// Get node capacity
		cpuCapacity := node.Status.Capacity.Cpu().AsDec().String()
		memoryCapacity := node.Status.Capacity.Memory().AsDec().String()

		// Calculate percentages
		cpuUsagePercent := calculateCPUPercentage(cpuUsage, cpuCapacity)
		memoryUsagePercent := calculateMemoryPercentage(memoryUsage, memoryCapacity)

		nodes = append(nodes, types.Node{
			Name:               node.Name,
			CPUUsage:           cpuUsage,
			MemoryUsage:        memoryUsage,
			CPUCapacity:        cpuCapacity,
			MemoryCapacity:     memoryCapacity,
			CPUUsagePercent:    cpuUsagePercent,
			MemoryUsagePercent: memoryUsagePercent,
			Condition:          getNodeCondition(&node),
			ConditionStatus:    getNodeConditionStatus(&node),
			Status:             string(node.Status.Phase),
		})
	}

	return nodes, nil
}

// calculateCPUPercentage calculates CPU usage percentage
func calculateCPUPercentage(usage, capacity string) float64 {
	usageValue := parseCPUValue(usage)
	capacityValue := parseCPUValue(capacity)

	if capacityValue == 0 {
		return 0
	}

	return (usageValue / capacityValue) * 100
}

// calculateMemoryPercentage calculates memory usage percentage
func calculateMemoryPercentage(usage, capacity string) float64 {
	usageValue := parseMemoryValue(usage)
	capacityValue := parseMemoryValue(capacity)

	if capacityValue == 0 {
		return 0
	}

	return (usageValue / capacityValue) * 100
}

// parseCPUValue converts CPU string to float64 (handles "100m" format)
func parseCPUValue(cpuStr string) float64 {
	var value float64
	var unit string
	_, err := fmt.Sscanf(cpuStr, "%f%s", &value, &unit)
	if err != nil {
		// Try parsing as plain number
		_, err := fmt.Sscanf(cpuStr, "%f", &value)
		if err != nil {
			return 0
		}
		return value
	}

	// Convert millicores to cores
	if unit == "m" {
		return value / 1000
	}

	return value
}

// parseMemoryValue converts memory string to float64 (handles "512Mi", "1Gi" format)
func parseMemoryValue(memoryStr string) float64 {
	var value float64
	var unit string
	_, err := fmt.Sscanf(memoryStr, "%f%s", &value, &unit)
	if err != nil {
		// Try parsing as plain number
		_, err := fmt.Sscanf(memoryStr, "%f", &value)
		if err != nil {
			return 0
		}
		return value
	}

	// Convert to bytes
	switch unit {
	case "Ki":
		return value * 1024
	case "Mi":
		return value * 1024 * 1024
	case "Gi":
		return value * 1024 * 1024 * 1024
	case "Ti":
		return value * 1024 * 1024 * 1024 * 1024
	case "Pi":
		return value * 1024 * 1024 * 1024 * 1024 * 1024
	case "k", "K":
		return value * 1000
	case "m", "M":
		return value * 1000 * 1000
	case "g", "G":
		return value * 1000 * 1000 * 1000
	case "t", "T":
		return value * 1000 * 1000 * 1000 * 1000
	case "p", "P":
		return value * 1000 * 1000 * 1000 * 1000 * 1000
	default:
		return value
	}
}

// collectPods collects pod data for a specific namespace
func (a *Agent) collectPods(ctx context.Context, namespace string) ([]types.Pod, error) {
	podList, err := a.k8sClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace %s: %v", namespace, err)
	}

	pods := make([]types.Pod, 0, len(podList.Items))
	for _, pod := range podList.Items {
		pods = append(pods, types.Pod{
			Name:         pod.Name,
			Namespace:    pod.Namespace,
			Status:       string(pod.Status.Phase),
			RestartCount: getPodRestartCount(&pod),
		})
	}

	return pods, nil
}

// collectServices collects service data for a specific namespace
func (a *Agent) collectServices(ctx context.Context, namespace string) ([]types.Service, error) {
	serviceList, err := a.k8sClient.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list services in namespace %s: %v", namespace, err)
	}

	services := make([]types.Service, 0, len(serviceList.Items))
	for _, service := range serviceList.Items {
		services = append(services, types.Service{
			Name: service.Name,
			Type: string(service.Spec.Type),
		})
	}

	return services, nil
}

// collectDeployments collects deployment data for a specific namespace
func (a *Agent) collectDeployments(ctx context.Context, namespace string) ([]types.Deployment, error) {
	deploymentList, err := a.k8sClient.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments in namespace %s: %v", namespace, err)
	}

	deployments := make([]types.Deployment, 0, len(deploymentList.Items))
	for _, deployment := range deploymentList.Items {
		replicas := int32(0)
		if deployment.Spec.Replicas != nil {
			replicas = *deployment.Spec.Replicas
		}
		deployments = append(deployments, types.Deployment{
			Name:     deployment.Name,
			Replicas: replicas,
		})
	}

	return deployments, nil
}

// shouldCollectResource checks if a resource type should be collected based on configuration
func (a *Agent) shouldCollectResource(resourceType string) bool {
	for _, resource := range a.config.Kubernetes.Resources {
		if resource == resourceType {
			return true
		}
	}
	return false
}

// getNodeCondition returns the primary condition of a node
func getNodeCondition(node *v1.Node) string {
	for _, condition := range node.Status.Conditions {
		if condition.Type == v1.NodeReady {
			return string(condition.Type)
		}
	}
	return "Unknown"
}

// getNodeConditionStatus returns the status of the primary condition
func getNodeConditionStatus(node *v1.Node) string {
	for _, condition := range node.Status.Conditions {
		if condition.Type == v1.NodeReady {
			return string(condition.Status)
		}
	}
	return "Unknown"
}

// getPodRestartCount returns the total restart count for a pod
func getPodRestartCount(pod *v1.Pod) int32 {
	var restarts int32
	for _, cs := range pod.Status.ContainerStatuses {
		restarts += cs.RestartCount
	}
	return restarts
}

// func (a *Agent) PrintHistory() {
// 	a.detector.PrintHistory()
// }

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
	// Update Prometheus metrics with current cluster state
	a.metrics.UpdateMetrics(a.state)

	anomalies := a.detector.DetectAnomalies(a.state)

	// Record anomalies in Prometheus
	for _, anomaly := range anomalies {
		a.metrics.RecordAnomaly(anomaly)
	}

	// Store anomalies in vector database if enabled
	if a.config.Storage.StoreAlerts {
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
		}
	}

	// Send notifications if severity is high enough and notifications are enabled
	if a.config.Notification.Enabled {
		for _, anomaly := range anomalies {
			if shouldNotify(anomaly, a.config.Notification.MinSeverity) {
				err := a.notifier.Notify(anomaly)
				if err != nil {
					log.Printf("Failed to send notification for anomaly: %v", err)
				}
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
		fmt.Printf("  - %s: CPU=%.2f%%, Memory=%.2f%%, Condition=%s, ConditionStatus=%s\n",
			node.Name, node.CPUUsagePercent, node.MemoryUsagePercent, node.Condition, node.ConditionStatus)
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
		fmt.Printf("  - [%s] %s (%s - %s): %s\n",
			anomaly.Severity, anomaly.Type, anomaly.Resource, anomaly.Namespace, anomaly.Description)
	}
}

// StartMetricsServer starts the Prometheus metrics server
func (a *Agent) StartMetricsServer() {
	a.metricsServer.StartAsync()
}
