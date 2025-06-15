package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/rodgon/valkyrie/pkg/anomaly"
	"github.com/rodgon/valkyrie/pkg/embedding"
	"github.com/rodgon/valkyrie/pkg/notification"
	"github.com/rodgon/valkyrie/pkg/storage"
	"github.com/rodgon/valkyrie/pkg/types"
)

// Agent represents our learning agent
type Agent struct {
	clientset          *kubernetes.Clientset
	metricsClient      *metricsv.Clientset
	state              types.ClusterState
	observations       []types.Observation
	learningRate       float64
	detector           *anomaly.Detector
	notifier           notification.Notifier
	notificationConfig *notification.NotificationConfig
	qdrantClient       *storage.QdrantClient
	embeddingModel     embedding.Model
}

// NewAgent creates a new Kubernetes agent
func NewAgent(kubeconfig string) (*Agent, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("error building kubeconfig: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating kubernetes client: %v", err)
	}

	metricsClient, err := metricsv.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating metrics client: %v", err)
	}

	// Load notification config
	notificationConfig := &notification.NotificationConfig{
		Enabled:         false,
		Type:            "alertmanager",
		MinSeverity:     "High",
		AlertManagerURL: "http://localhost:9093",
		AlertLabels: map[string]string{
			"source":  "k8s-agent",
			"cluster": "default",
		},
	}

	// Create appropriate notifier
	var notifier notification.Notifier
	if notificationConfig.Enabled {
		switch notificationConfig.Type {
		case "alertmanager":
			notifier = &notification.AlertmanagerNotifier{
				URL:           notificationConfig.AlertManagerURL,
				DefaultLabels: notificationConfig.AlertLabels,
			}
		case "slack":
			notifier = &notification.SlackNotifier{
				WebhookURL: notificationConfig.Endpoint,
				Channel:    notificationConfig.Channel,
			}
		case "email":
			notifier = &notification.EmailNotifier{
				SMTPHost:   "smtp.example.com",
				SMTPPort:   587,
				Recipients: notificationConfig.Recipients,
			}
		case "webhook":
			notifier = &notification.WebhookNotifier{
				Endpoint: notificationConfig.Endpoint,
			}
		}
	}

	// Initialize Qdrant client
	qdrantClient := storage.NewQdrantClient("http://localhost:6333", "k8s-alerts")

	// Initialize embedding model
	embeddingModel := embedding.NewSimpleModel(384) // Using 384 dimensions

	return &Agent{
		clientset:     clientset,
		metricsClient: metricsClient,
		learningRate:  0.1,
		state: types.ClusterState{
			Resources: make(map[string]types.ResourceState),
		},
		detector:           anomaly.NewDetector(),
		notifier:           notifier,
		notificationConfig: notificationConfig,
		qdrantClient:       qdrantClient,
		embeddingModel:     embeddingModel,
	}, nil
}

// ObserveCluster collects current cluster state
func (a *Agent) ObserveCluster() error {
	ctx := context.TODO()
	a.state.Timestamp = time.Now()

	// Get namespaces
	namespaces, err := a.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error listing namespaces: %v", err)
	}
	a.state.Namespaces = make([]string, len(namespaces.Items))
	for i, ns := range namespaces.Items {
		a.state.Namespaces[i] = ns.Name
	}

	// Get nodes
	nodes, err := a.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error listing nodes: %v", err)
	}

	// Get node metrics
	nodeMetrics, err := a.metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error getting node metrics: %v", err)
	}

	// Create node state map for quick lookup
	nodeMetricsMap := make(map[string]metricsv1beta1.NodeMetrics)
	for _, metric := range nodeMetrics.Items {
		nodeMetricsMap[metric.Name] = metric
	}

	// Process nodes
	a.state.Nodes = make([]types.NodeState, len(nodes.Items))
	for i, node := range nodes.Items {
		metrics := nodeMetricsMap[node.Name]
		a.state.Nodes[i] = types.NodeState{
			Name:            node.Name,
			Status:          string(node.Status.Phase),
			CPUCapacity:     node.Status.Capacity.Cpu().String(),
			MemoryCapacity:  node.Status.Capacity.Memory().String(),
			LastObservation: time.Now(),
		}
		if metrics.Name != "" {
			a.state.Nodes[i].CPUUsage = metrics.Usage.Cpu().String()
			a.state.Nodes[i].MemoryUsage = metrics.Usage.Memory().String()
		}
	}

	// Process resources in each namespace
	for _, ns := range a.state.Namespaces {
		resourceState := types.ResourceState{}

		// Get pods
		pods, err := a.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Printf("Error listing pods in namespace %s: %v", ns, err)
			continue
		}
		resourceState.Pods = make([]types.PodState, len(pods.Items))
		for i, pod := range pods.Items {
			// Calculate total restart count
			var restartCount int32
			for _, containerStatus := range pod.Status.ContainerStatuses {
				restartCount += containerStatus.RestartCount
			}

			resourceState.Pods[i] = types.PodState{
				Name:         pod.Name,
				Status:       string(pod.Status.Phase),
				Namespace:    ns,
				Age:          time.Since(pod.CreationTimestamp.Time),
				RestartCount: restartCount,
			}
		}

		// Get services
		services, err := a.clientset.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Printf("Error listing services in namespace %s: %v", ns, err)
			continue
		}
		resourceState.Services = make([]types.ServiceState, len(services.Items))
		for i, svc := range services.Items {
			resourceState.Services[i] = types.ServiceState{
				Name:      svc.Name,
				Type:      string(svc.Spec.Type),
				Namespace: ns,
			}
		}

		// Get deployments
		deployments, err := a.clientset.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Printf("Error listing deployments in namespace %s: %v", ns, err)
			continue
		}
		resourceState.Deployments = make([]types.DeploymentState, len(deployments.Items))
		for i, dep := range deployments.Items {
			resourceState.Deployments[i] = types.DeploymentState{
				Name:      dep.Name,
				Replicas:  *dep.Spec.Replicas,
				Namespace: ns,
			}
		}

		a.state.Resources[ns] = resourceState
	}

	return nil
}

// Learn processes the current state and updates the agent's knowledge
func (a *Agent) Learn() {
	// Calculate reward based on cluster health
	reward := a.calculateReward()

	// Create new observation
	observation := types.Observation{
		Timestamp: time.Now(),
		Action:    "observe",
		State:     a.state,
		Reward:    reward,
	}

	// Add to observations
	a.observations = append(a.observations, observation)

	// Save observations to file for persistence
	a.saveObservations()
}

// calculateReward calculates a reward based on cluster health
func (a *Agent) calculateReward() float64 {
	var reward float64

	// Check node health
	for _, node := range a.state.Nodes {
		if node.Status == "Ready" {
			reward += 1.0
		} else {
			reward -= 2.0
		}
	}

	// Check pod health
	for _, resourceState := range a.state.Resources {
		for _, pod := range resourceState.Pods {
			if pod.Status == "Running" {
				reward += 0.5
			} else if pod.Status == "Failed" {
				reward -= 1.0
			}
		}
	}

	return reward
}

// saveObservations saves the learning observations to a file
func (a *Agent) saveObservations() {
	data, err := json.MarshalIndent(a.observations, "", "  ")
	if err != nil {
		log.Printf("Error marshaling observations: %v", err)
		return
	}

	err = os.WriteFile("k8s_agent_observations.json", data, 0644)
	if err != nil {
		log.Printf("Error saving observations: %v", err)
	}
}

// PrintState prints the current cluster state
func (a *Agent) PrintState() {
	fmt.Printf("\n=== Cluster State at %s ===\n", a.state.Timestamp.Format(time.RFC3339))

	fmt.Println("\nNamespaces:")
	for _, ns := range a.state.Namespaces {
		fmt.Printf("  - %s\n", ns)
	}

	fmt.Println("\nNodes:")
	for _, node := range a.state.Nodes {
		fmt.Printf("  - %s\n", node.Name)
		fmt.Printf("    Status: %s\n", node.Status)
		fmt.Printf("    CPU Usage: %s/%s\n", node.CPUUsage, node.CPUCapacity)
		fmt.Printf("    Memory Usage: %s/%s\n", node.MemoryUsage, node.MemoryCapacity)
	}

	fmt.Println("\nResources by Namespace:")
	for ns, resources := range a.state.Resources {
		fmt.Printf("\nNamespace: %s\n", ns)
		fmt.Printf("  Pods: %d\n", len(resources.Pods))
		fmt.Printf("  Services: %d\n", len(resources.Services))
		fmt.Printf("  Deployments: %d\n", len(resources.Deployments))
	}
}

// collectEvents collects relevant events for a resource
func (a *Agent) collectEvents(resourceName, namespace string, duration time.Duration) []types.Event {
	ctx := context.TODO()
	events, err := a.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", resourceName),
	})
	if err != nil {
		log.Printf("Error collecting events: %v", err)
		return nil
	}

	var relevantEvents []types.Event
	cutoffTime := time.Now().Add(-duration)

	for _, event := range events.Items {
		if event.LastTimestamp.Time.After(cutoffTime) {
			relevantEvents = append(relevantEvents, types.Event{
				Type:      event.Type,
				Reason:    event.Reason,
				Message:   event.Message,
				Timestamp: event.LastTimestamp.Time,
				Object:    event.InvolvedObject.Name,
				Namespace: event.Namespace,
			})
		}
	}

	return relevantEvents
}

// enrichAnomaly adds relevant events to the anomaly description
func (a *Agent) enrichAnomaly(anomaly types.Anomaly) types.Anomaly {
	events := a.collectEvents(anomaly.Resource, anomaly.Namespace, 5*time.Minute)

	if len(events) > 0 {
		anomaly.Description += "\n\nRecent Events:"
		for _, event := range events {
			anomaly.Description += fmt.Sprintf("\n- [%s] %s: %s",
				event.Timestamp.Format(time.RFC3339),
				event.Reason,
				event.Message)
		}
	}

	return anomaly
}

// generateAlertEmbedding generates a vector embedding for an alert
func (a *Agent) generateAlertEmbedding(anomaly types.Anomaly, events []types.Event) []float32 {
	// Combine text for embedding
	text := fmt.Sprintf("%s: %s", anomaly.Type, anomaly.Description)
	for _, event := range events {
		text += fmt.Sprintf(" Event: %s - %s", event.Reason, event.Message)
	}

	// Generate embedding
	vector, err := a.embeddingModel.Encode(text)
	if err != nil {
		log.Printf("Error generating embedding: %v", err)
		// Return zero vector as fallback
		return make([]float32, 384)
	}

	return vector
}

// shouldNotify determines if an anomaly should trigger a notification
func (a *Agent) shouldNotify(anomaly types.Anomaly) bool {
	severityLevels := map[string]int{
		"Low":    1,
		"Medium": 2,
		"High":   3,
	}

	configLevel := severityLevels[a.notificationConfig.MinSeverity]
	anomalyLevel := severityLevels[anomaly.Severity]

	return anomalyLevel >= configLevel
}

// DetectAnomalies checks for anomalies in the current cluster state
func (a *Agent) DetectAnomalies() []types.Anomaly {
	anomalies := a.detector.DetectAnomalies(a.state)

	// Enrich anomalies with events and store in Qdrant
	for i := range anomalies {
		// Enrich with events
		events := a.collectEvents(anomalies[i].Resource, anomalies[i].Namespace, 5*time.Minute)
		anomalies[i] = a.enrichAnomaly(anomalies[i])

		// Generate vector embedding for the alert
		vector := a.generateAlertEmbedding(anomalies[i], events)

		// Store in Qdrant
		if err := a.qdrantClient.StoreAlert(anomalies[i], events, vector); err != nil {
			log.Printf("Error storing alert in Qdrant: %v", err)
		}

		// Send notification if severity matches
		if a.shouldNotify(anomalies[i]) {
			if err := a.notifier.Notify(anomalies[i]); err != nil {
				log.Printf("Error sending notification: %v", err)
			}
		}
	}

	return anomalies
}

// PrintAnomalies prints detected anomalies
func (a *Agent) PrintAnomalies(anomalies []types.Anomaly) {
	if len(anomalies) == 0 {
		fmt.Println("\nNo anomalies detected.")
		return
	}

	fmt.Println("\n=== Detected Anomalies ===")
	for _, anomaly := range anomalies {
		fmt.Printf("\nType: %s\n", anomaly.Type)
		fmt.Printf("Resource: %s\n", anomaly.Resource)
		if anomaly.Namespace != "" {
			fmt.Printf("Namespace: %s\n", anomaly.Namespace)
		}
		fmt.Printf("Severity: %s\n", anomaly.Severity)
		fmt.Printf("Description: %s\n", anomaly.Description)
		fmt.Printf("Timestamp: %s\n", anomaly.Timestamp.Format(time.RFC3339))
		if anomaly.Value != 0 {
			fmt.Printf("Value: %.2f\n", anomaly.Value)
			fmt.Printf("Threshold: %.2f\n", anomaly.Threshold)
		}
	}
}
