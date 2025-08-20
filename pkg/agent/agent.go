package agent

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"text/template"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

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

// formatAnomalyForEncoding formats an anomaly as text for vector encoding using template
func formatAnomalyForEncoding(anomaly types.Anomaly, cfg *config.Config) (string, error) {
	return formatAnomaly(anomaly, cfg.Formatting.AnomalyEncodingTemplate)
}

// formatAnomalyForDisplay formats an anomaly for console display using template
func formatAnomalyForDisplay(anomaly types.Anomaly, cfg *config.Config) (string, error) {
	return formatAnomaly(anomaly, cfg.Formatting.AnomalyDisplayTemplate)
}

// safeAnomalyData wraps an anomaly with safe defaults for template execution
type safeAnomalyData struct {
	types.Anomaly
}

// Safe methods for template execution with fallbacks
func (s safeAnomalyData) SafeClusterName() string {
	if s.ClusterName == "" {
		return "unknown-cluster"
	}
	return s.ClusterName
}

func (s safeAnomalyData) SafeSeverity() string {
	if s.Severity == "" {
		return "Medium"
	}
	return s.Severity
}

func (s safeAnomalyData) SafeType() string {
	if s.Type == "" {
		return "UnknownAnomaly"
	}
	return s.Type
}

func (s safeAnomalyData) SafeResourceType() string {
	if s.ResourceType == "" {
		return "resource"
	}
	return s.ResourceType
}

func (s safeAnomalyData) SafeResource() string {
	if s.Resource == "" {
		return "unknown-resource"
	}
	return s.Resource
}

func (s safeAnomalyData) SafeNamespace() string {
	if s.Namespace == "" {
		return "default"
	}
	return s.Namespace
}

func (s safeAnomalyData) SafeNodeName() string {
	return s.NodeName // This can be empty for cluster events
}

func (s safeAnomalyData) SafeDescription() string {
	if s.Description == "" {
		return "No description available"
	}
	return s.Description
}

func (s safeAnomalyData) HasNodeName() bool {
	return s.NodeName != ""
}

// formatAnomaly is a generic function to format an anomaly using a template with safe defaults.
func formatAnomaly(anomaly types.Anomaly, tplt string) (string, error) {
	// Define template functions for additional safety
	funcMap := template.FuncMap{
		"default": func(value, defaultValue string) string {
			if value == "" {
				return defaultValue
			}
			return value
		},
		"nonEmpty": func(value string) bool {
			return strings.TrimSpace(value) != ""
		},
	}

	tmpl, err := template.New("anomaly").Funcs(funcMap).Parse(tplt)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %v", err)
	}

	// Wrap anomaly with safe accessors
	safeData := safeAnomalyData{Anomaly: anomaly}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, safeData); err != nil {
		return "", fmt.Errorf("failed to execute template: %v", err)
	}

	result := buf.String()
	trimmedResult := strings.TrimSpace(result)

	// Final safety check - if the result is still empty or contains only template artifacts
	if trimmedResult == "" || trimmedResult == "Anomaly on cluster  of severity  and type " {
		// Generate a minimal fallback description
		result = fmt.Sprintf("Anomaly on cluster %s of severity %s and type %s in %s name %s on namespace %s, alert description: %s",
			safeData.SafeClusterName(),
			safeData.SafeSeverity(),
			safeData.SafeType(),
			safeData.SafeResourceType(),
			safeData.SafeResource(),
			safeData.SafeNamespace(),
			safeData.SafeDescription(),
		)
	}
	// log.Printf("result: %+v\n", result)
	return result, nil
}

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
	clusterID     string // Cluster ID for multi-cluster mode
	clusterName   string // Cluster name for multi-cluster mode
}

// NewAgent creates a new agent instance
func NewAgent(cfg *config.Config) (*Agent, error) {
	// Use the first cluster config (single-cluster mode)
	if len(cfg.Clusters) == 0 {
		return nil, fmt.Errorf("no clusters defined in config")
	}
	clusterCfg := cfg.Clusters[0]

	// Load kubeconfig
	kubeconfig := clusterCfg.Kubeconfig
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
			Type:       storage.StorageType(cfg.Storage.Type),
			URL:        cfg.Storage.Qdrant.URL,
			Password:   cfg.Storage.Redis.Password,
			DB:         cfg.Storage.Redis.DB,
			Collection: cfg.Storage.Qdrant.Collection,
			VectorSize: cfg.Storage.Qdrant.VectorSize,
			Distance:   cfg.Storage.Qdrant.DistanceMetric,
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

// NewAgentWithoutMetrics creates a new agent instance without metrics, storage, notifier, and model
// These components will be set by the multi-cluster agent
func NewAgentWithoutMetrics(cfg *config.Config) (*Agent, error) {
	// Use the first cluster config (single-cluster mode)
	if len(cfg.Clusters) == 0 {
		return nil, fmt.Errorf("no clusters defined in config")
	}
	clusterCfg := cfg.Clusters[0]

	// Load kubeconfig
	kubeconfig := clusterCfg.Kubeconfig
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

	return &Agent{
		k8sClient:    clientset,
		restConfig:   config,
		detector:     detector,
		config:       cfg,
		observations: make([]types.Observation, 0),
		// Note: metrics, storage, notifier, model, and metricsServer will be set by the caller
	}, nil
}

// SetClusterInfo sets the cluster information for multi-cluster mode
func (a *Agent) SetClusterInfo(clusterID, clusterName string) {
	a.clusterID = clusterID
	a.clusterName = clusterName
}

// ObserveCluster collects the current state of the cluster
func (a *Agent) ObserveCluster() error {
	return a.ObserveClusterWithContext(context.Background())
}

// ObserveClusterWithContext collects the current state of the cluster with context cancellation support
func (a *Agent) ObserveClusterWithContext(ctx context.Context) error {

	// Create metrics client
	metricsClient, err := metricsv.NewForConfig(a.restConfig)
	if err != nil {
		return fmt.Errorf("failed to create metrics client: %v", err)
	}

	// Initialize cluster state
	var nodes []types.Node
	var resources = make(map[string]types.ResourceList)
	var events []types.ClusterEvent
	var pvs []types.PersistentVolume

	// Collect namespaces (always needed for resource organization)
	nsList, err := a.k8sClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %v", err)
	}

	// Collect cluster events if configured
	if a.shouldCollectResource("events") {
		events, err = a.collectEvents(ctx)
		if err != nil {
			log.Printf("Warning: failed to collect events: %v", err)
		}
	}

	// Build node-to-namespaces mapping by collecting all pods first
	// This is always done regardless of pod monitoring configuration
	nodeNamespaces := make(map[string]map[string]bool) // node -> set of namespaces

	// Always collect minimal pod information for node mapping
	for _, ns := range nsList.Items {
		// Get pods for node mapping (minimal collection)
		podList, err := a.k8sClient.CoreV1().Pods(ns.Name).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Printf("Warning: failed to list pods in namespace %s: %v", ns.Name, err)
			continue
		}

		// Build node-to-namespaces mapping from pods
		for _, pod := range podList.Items {
			if pod.Spec.NodeName != "" {
				if nodeNamespaces[pod.Spec.NodeName] == nil {
					nodeNamespaces[pod.Spec.NodeName] = make(map[string]bool)
				}
				nodeNamespaces[pod.Spec.NodeName][ns.Name] = true
			}
		}

		// Collect full resource data per namespace if configured
		resourceList := types.ResourceList{}

		// Collect pods if configured (reuse already fetched podList to avoid duplicate API calls)
		if a.shouldCollectResource("pods") {
			pods := make([]types.Pod, 0, len(podList.Items))
			for _, pod := range podList.Items {
				pods = append(pods, types.Pod{
					Name:         pod.Name,
					Namespace:    pod.Namespace,
					NodeName:     pod.Spec.NodeName,
					Status:       string(pod.Status.Phase),
					RestartCount: getPodRestartCount(&pod),
				})
			}
			resourceList.Pods = pods
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

		// Collect persistent volume claims if configured
		if a.shouldCollectResource("persistentvolumeclaims") {
			pvcs, err := a.collectPVCs(ctx, ns.Name)
			if err != nil {
				log.Printf("Warning: failed to collect PVCs in namespace %s: %v", ns.Name, err)
			} else if len(pvcs) > 0 {
				resourceList.PersistentVolumeClaims = pvcs
			}
		}

		// Only add namespace to resources if we collected any data
		if len(resourceList.Pods) > 0 || len(resourceList.Services) > 0 || len(resourceList.Deployments) > 0 {
			resources[ns.Name] = resourceList
		}
	}

	// After per-namespace collection, collect cluster-scoped PVs if configured
	if a.shouldCollectResource("persistentvolumes") {
		var err error
		pvs, err = a.collectPVs(ctx)
		if err != nil {
			log.Printf("Warning: failed to collect PVs: %v", err)
		}
	}

	// Convert nodeNamespaces map to the format expected by collectNodes
	nodeNamespacesList := make(map[string][]string)
	for nodeName, namespaceSet := range nodeNamespaces {
		namespaces := make([]string, 0, len(namespaceSet))
		for namespace := range namespaceSet {
			namespaces = append(namespaces, namespace)
		}
		nodeNamespacesList[nodeName] = namespaces
	}

	// Collect node data if configured
	if a.shouldCollectResource("nodes") {
		nodes, err = a.collectNodes(ctx, metricsClient, nodeNamespacesList)
		if err != nil {
			return fmt.Errorf("failed to collect nodes: %v", err)
		}
	}

	// Build namespace names list
	nsNames := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		nsNames = append(nsNames, ns.Name)
	}

	// Set cluster information from agent fields or fall back to config
	clusterID := a.clusterID
	clusterName := a.clusterName
	if clusterID == "" && len(a.config.Clusters) > 0 {
		clusterID = a.config.Clusters[0].ID
	}
	if clusterName == "" && len(a.config.Clusters) > 0 {
		clusterName = a.config.Clusters[0].Name
	}

	a.state = types.ClusterState{
		ClusterID:         clusterID,
		ClusterName:       clusterName,
		Namespaces:        nsNames,
		Nodes:             nodes,
		Resources:         resources,
		Events:            events,
		PersistentVolumes: pvs,
	}
	return nil
}

// collectNodes collects node data including metrics
func (a *Agent) collectNodes(ctx context.Context, metricsClient *metricsv.Clientset, nodeNamespaces map[string][]string) ([]types.Node, error) {
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

		// Get namespaces running on this node
		namespaces := nodeNamespaces[node.Name]
		if namespaces == nil {
			namespaces = []string{} // Ensure we have an empty slice instead of nil
		}

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
			Namespaces:         namespaces,
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
func (a *Agent) collectPods(ctx context.Context, namespace string) ([]types.Pod, map[string]string, error) {
	podList, err := a.k8sClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list pods in namespace %s: %v", namespace, err)
	}

	pods := make([]types.Pod, 0, len(podList.Items))
	podToNode := make(map[string]string) // pod name -> node name

	for _, pod := range podList.Items {
		nodeName := pod.Spec.NodeName
		pods = append(pods, types.Pod{
			Name:         pod.Name,
			Namespace:    pod.Namespace,
			NodeName:     nodeName,
			Status:       string(pod.Status.Phase),
			RestartCount: getPodRestartCount(&pod),
		})

		// Store the node name for this pod
		if nodeName != "" {
			podToNode[pod.Name] = nodeName
		}
	}

	return pods, podToNode, nil
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

// collectPVCs collects PersistentVolumeClaims for a specific namespace
func (a *Agent) collectPVCs(ctx context.Context, namespace string) ([]types.PersistentVolumeClaim, error) {
	pvcList, err := a.k8sClient.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list PVCs in namespace %s: %v", namespace, err)
	}

	pvcs := make([]types.PersistentVolumeClaim, 0, len(pvcList.Items))
	for _, pvc := range pvcList.Items {
		storageClass := ""
		if pvc.Spec.StorageClassName != nil {
			storageClass = *pvc.Spec.StorageClassName
		}
		// Access modes
		accessModes := make([]string, 0, len(pvc.Spec.AccessModes))
		for _, m := range pvc.Spec.AccessModes {
			accessModes = append(accessModes, string(m))
		}
		// Requested storage
		requested := ""
		if qty, ok := pvc.Spec.Resources.Requests[v1.ResourceStorage]; ok {
			requested = qty.String()
		}

		pvcs = append(pvcs, types.PersistentVolumeClaim{
			Name:             pvc.Name,
			Namespace:        pvc.Namespace,
			Status:           string(pvc.Status.Phase),
			VolumeName:       pvc.Spec.VolumeName,
			StorageClassName: storageClass,
			AccessModes:      accessModes,
			RequestedStorage: requested,
		})
	}

	return pvcs, nil
}

// collectPVs collects PersistentVolumes cluster-wide
func (a *Agent) collectPVs(ctx context.Context) ([]types.PersistentVolume, error) {
	pvList, err := a.k8sClient.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list PVs: %v", err)
	}

	pvs := make([]types.PersistentVolume, 0, len(pvList.Items))
	for _, pv := range pvList.Items {
		// Capacity
		capacity := ""
		if qty, ok := pv.Spec.Capacity[v1.ResourceStorage]; ok {
			capacity = qty.String()
		}
		// Access modes
		accessModes := make([]string, 0, len(pv.Spec.AccessModes))
		for _, m := range pv.Spec.AccessModes {
			accessModes = append(accessModes, string(m))
		}
		// Volume mode
		volumeMode := ""
		if pv.Spec.VolumeMode != nil {
			volumeMode = string(*pv.Spec.VolumeMode)
		}
		// Claim ref
		claimNamespace := ""
		claimName := ""
		if pv.Spec.ClaimRef != nil {
			claimNamespace = pv.Spec.ClaimRef.Namespace
			claimName = pv.Spec.ClaimRef.Name
		}

		pvs = append(pvs, types.PersistentVolume{
			Name:             pv.Name,
			Status:           string(pv.Status.Phase),
			Capacity:         capacity,
			StorageClassName: pv.Spec.StorageClassName,
			AccessModes:      accessModes,
			ReclaimPolicy:    string(pv.Spec.PersistentVolumeReclaimPolicy),
			VolumeMode:       volumeMode,
			ClaimNamespace:   claimNamespace,
			ClaimName:        claimName,
		})
	}

	return pvs, nil
}

// shouldCollectResource checks if a resource type should be collected based on configuration
func (a *Agent) shouldCollectResource(resourceType string) bool {
	if len(a.config.Clusters) == 0 {
		return false
	}
	// Normalize desired type and provide aliases
	wanted := strings.ToLower(resourceType)
	for _, resource := range a.config.Clusters[0].Resources {
		r := strings.ToLower(resource)
		if r == wanted {
			return true
		}
		// Aliases for PVs and PVCs
		if wanted == "persistentvolumes" && (r == "pv" || r == "persistentvolume" || r == "persistentvolumes") {
			return true
		}
		if wanted == "persistentvolumeclaims" && (r == "pvc" || r == "persistentvolumeclaim" || r == "persistentvolumeclaims") {
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
	// Update Prometheus metrics with current cluster state (if metrics exist)
	if a.metrics != nil {
		a.metrics.UpdateMetrics(a.state)
	}

	anomalies := a.detector.DetectAnomalies(a.state)

	// Record anomalies in Prometheus (if metrics exist)
	if a.metrics != nil {
		for _, anomaly := range anomalies {
			a.metrics.RecordAnomaly(anomaly)
		}
	}

	// Store anomalies in vector database if enabled and storage exists
	if a.config.Storage.StoreAlerts && a.storage != nil && a.model != nil {
		for _, anomaly := range anomalies {
			// Generate embedding for the anomaly
			text, err := formatAnomalyForEncoding(anomaly, a.config)
			if err != nil {
				log.Printf("Failed to format anomaly for encoding: %v", err)
				continue
			}

			// Validate that the formatted text is not empty or whitespace-only
			if strings.TrimSpace(text) == "" {
				log.Printf("Skipping embedding for anomaly with empty formatted text: %+v", anomaly)
				continue
			}

			vector, err := a.model.Encode(text)
			if err != nil {
				log.Printf("Failed to generate embedding for anomaly: %v (text length: %d, text: '%.200s')",
					err, len(text), text)
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
	if a.config.Notification.Enabled && a.notifier != nil {
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
		namespacesStr := "none"
		if len(node.Namespaces) > 0 {
			namespacesStr = strings.Join(node.Namespaces, ", ")
		}
		fmt.Printf("  - %s: CPU=%.2f%%, Memory=%.2f%%, Condition=%s, ConditionStatus=%s, Namespaces=[%s]\n",
			node.Name, node.CPUUsagePercent, node.MemoryUsagePercent, node.Condition, node.ConditionStatus, namespacesStr)
	}

	// Print events if any
	if len(a.state.Events) > 0 {
		fmt.Printf("Events: %d\n", len(a.state.Events))
		// Show only the most recent events (last 10)
		start := 0
		if len(a.state.Events) > 10 {
			start = len(a.state.Events) - 10
		}
		for _, event := range a.state.Events[start:] {
			fmt.Printf("  - [%s] %s/%s: %s (count: %d)\n",
				event.Severity, event.Namespace, event.Resource, event.Message, event.Count)
		}
	} else {
		fmt.Printf("Events: 0\n")
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
		formatted, err := formatAnomalyForDisplay(anomaly, a.config)
		if err != nil {
			log.Printf("Failed to format anomaly for display: %v", err)
			// Fallback to simple format
			fmt.Printf("  - [%s] %s/%s (%s - %s): %s\n",
				anomaly.Severity, anomaly.ClusterName, anomaly.Resource, anomaly.Type, anomaly.Namespace, anomaly.Description)
		} else {
			fmt.Print(formatted)
		}
	}
}

// StartMetricsServer starts the Prometheus metrics server
func (a *Agent) StartMetricsServer() {
	if a.metricsServer != nil {
		a.metricsServer.StartAsync()
	}
}

// collectEvents collects cluster events
func (a *Agent) collectEvents(ctx context.Context) ([]types.ClusterEvent, error) {
	// Get events from all namespaces
	eventList, err := a.k8sClient.CoreV1().Events("").List(ctx, metav1.ListOptions{
		Limit: 1000, // Limit to prevent overwhelming the system
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %v", err)
	}

	events := make([]types.ClusterEvent, 0, len(eventList.Items))
	for _, event := range eventList.Items {
		// Convert Kubernetes event to our ClusterEvent type
		clusterEvent := types.ClusterEvent{
			Type:      event.Type,
			Reason:    event.Reason,
			Message:   event.Message,
			Timestamp: event.LastTimestamp.Time,
			Namespace: event.Namespace,
			Resource:  event.InvolvedObject.Name,
			Severity:  string(event.Type),
			Count:     event.Count,
		}
		events = append(events, clusterEvent)
	}

	return events, nil
}

// PrintConfig prints the current configuration
func (a *Agent) PrintConfig() {
	if len(a.config.Clusters) == 0 {
		fmt.Printf("No cluster configuration found\n")
		return
	}

	cluster := a.config.Clusters[0]
	fmt.Printf("Single Cluster Configuration:\n")
	fmt.Printf("Cluster: %s (%s)\n", cluster.Name, cluster.ID)
	fmt.Printf("Kubeconfig: %s\n", cluster.Kubeconfig)
	if cluster.Context != "" {
		fmt.Printf("Context: %s\n", cluster.Context)
	}
	if cluster.Namespace != "" {
		fmt.Printf("Namespace: %s\n", cluster.Namespace)
	} else {
		fmt.Printf("Namespace: all\n")
	}
	fmt.Printf("Resources: %v\n", cluster.Resources)
	if len(cluster.Labels) > 0 {
		fmt.Printf("Labels: %v\n", cluster.Labels)
	}

	fmt.Printf("\nAnomaly Detection:\n")
	fmt.Printf("  CPU Threshold: %.1f%%\n", a.config.AnomalyDetection.CPUThreshold)
	fmt.Printf("  Memory Threshold: %.1f%%\n", a.config.AnomalyDetection.MemoryThreshold)
	fmt.Printf("  Pod Restart Threshold: %d\n", a.config.AnomalyDetection.PodRestartThreshold)
	fmt.Printf("  Max History Size: %d\n", a.config.AnomalyDetection.MaxHistorySize)

	fmt.Printf("\nStorage:\n")
	fmt.Printf("  Type: %s\n", a.config.Storage.Type)
	fmt.Printf("  Store Alerts: %t\n", a.config.Storage.StoreAlerts)
	if a.config.Storage.Type == "qdrant" {
		fmt.Printf("  Qdrant URL: %s\n", a.config.Storage.Qdrant.URL)
		fmt.Printf("  Collection: %s\n", a.config.Storage.Qdrant.Collection)
	}

	fmt.Printf("\nEmbedding:\n")
	fmt.Printf("  Type: %s\n", a.config.Embedding.Type)
	fmt.Printf("  Dimension: %d\n", a.config.Embedding.Dimension)
	if a.config.Embedding.Type == "ollama" {
		fmt.Printf("  Ollama URL: %s\n", a.config.Embedding.Ollama.URL)
		fmt.Printf("  Model: %s\n", a.config.Embedding.Ollama.Model)
	}

	fmt.Printf("\nNotification:\n")
	fmt.Printf("  Enabled: %t\n", a.config.Notification.Enabled)
	if a.config.Notification.Enabled {
		fmt.Printf("  Type: %s\n", a.config.Notification.Type)
		fmt.Printf("  Min Severity: %s\n", a.config.Notification.MinSeverity)
	}

	fmt.Printf("\nObservation:\n")
	fmt.Printf("  Interval: %d seconds\n", a.config.ObservationInterval)
}
