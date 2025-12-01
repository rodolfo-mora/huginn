package types

import (
	"time"
)

// ClusterState represents the current state of the Kubernetes cluster
type ClusterState struct {
	ClusterID   string
	ClusterName string
	Namespaces  []string
	Nodes       []Node
	Resources   map[string]ResourceList
	Events      []ClusterEvent // Cluster-wide events
	// Cluster-scoped resources
	PersistentVolumes []PersistentVolume
}

// MultiClusterState represents the aggregated state of multiple clusters
type MultiClusterState struct {
	Clusters map[string]ClusterState
	Summary  ClusterSummary
}

// ClusterSummary represents a summary of cluster health and status
type ClusterSummary struct {
	TotalClusters     int
	HealthyClusters   int
	UnhealthyClusters int
	TotalNodes        int
	TotalAnomalies    int
	LastUpdated       time.Time
}

// Node represents a Kubernetes node
type Node struct {
	Name               string
	CPUUsage           string  // Raw CPU usage (e.g., "100m")
	MemoryUsage        string  // Raw memory usage (e.g., "512Mi")
	CPUCapacity        string  // Total CPU capacity (e.g., "4")
	MemoryCapacity     string  // Total memory capacity (e.g., "8Gi")
	CPUUsagePercent    float64 // Calculated CPU usage percentage
	MemoryUsagePercent float64 // Calculated memory usage percentage
	Condition          string
	ConditionStatus    string
	Status             string
	Namespaces         []string // Namespaces running on this node
}

// ResourceList represents a list of resources in a namespace
type ResourceList struct {
	Pods        []Pod
	Services    []Service
	Deployments []Deployment
	// Namespace-scoped storage resources
	PersistentVolumeClaims []PersistentVolumeClaim
}

// Pod represents a Kubernetes pod
type Pod struct {
	Name           string
	Namespace      string
	NodeName       string // Name of the Kubernetes node where this pod is running
	Status         string
	RestartCount   int32
	CPURequests    string // Effective CPU requests for the pod
	CPULimits      string // Effective CPU limits for the pod
	MemoryRequests string // Effective memory requests for the pod
	MemoryLimits   string // Effective memory limits for the pod
	State          string // State of the pod
}

// Service represents a Kubernetes service
type Service struct {
	Name string
	Type string
}

// Deployment represents a Kubernetes deployment
type Deployment struct {
	Name     string
	Replicas int32
}

// PersistentVolumeClaim represents a Kubernetes PVC
type PersistentVolumeClaim struct {
	Name             string
	Namespace        string
	Status           string
	VolumeName       string
	StorageClassName string
	AccessModes      []string
	RequestedStorage string
}

// PersistentVolume represents a Kubernetes PV
type PersistentVolume struct {
	Name             string
	Status           string
	Capacity         string
	StorageClassName string
	AccessModes      []string
	ReclaimPolicy    string
	VolumeMode       string
	ClaimNamespace   string
	ClaimName        string
}

// Anomaly represents a detected anomaly in the cluster
type Anomaly struct {
	ClusterID            string
	ClusterName          string
	Type                 string
	ResourceType         string // Type of Kubernetes resource (node, pod, service, deployment, event)
	Resource             string
	Namespace            string
	NodeName             string // Name of the Kubernetes node where this anomaly occurred
	NamespacesOnThisNode string
	Severity             string
	Description          string
	Value                float64
	Threshold            float64
	Timestamp            time.Time
	Labels               map[string]string
	Events               []Event
	Metadata             map[string]interface{}
}

// Event represents a Kubernetes event
type Event struct {
	Type      string
	Reason    string
	Message   string
	Timestamp time.Time
}

// Observation represents a historical observation of the cluster state
type Observation struct {
	ClusterID   string
	ClusterName string
	Timestamp   time.Time
	State       ClusterState
	Reward      float64
}

// AlertmanagerAlert represents an alert sent to Alertmanager
type AlertmanagerAlert struct {
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
}

// AlertmanagerPayload represents the payload sent to Alertmanager
type AlertmanagerPayload struct {
	Alerts []AlertmanagerAlert `json:"alerts"`
}

// ClusterEvent represents a Kubernetes cluster event
type ClusterEvent struct {
	Type      string
	Reason    string
	Message   string
	Timestamp time.Time
	Namespace string
	Resource  string
	Severity  string // Normal, Warning, Error
	Count     int32  // Number of times this event occurred
}
