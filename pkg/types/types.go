package types

import (
	"time"
)

// ClusterState represents the current state of the cluster
type ClusterState struct {
	Timestamp    time.Time
	Namespaces   []string
	Nodes        []NodeState
	Resources    map[string]ResourceState
	Observations []Observation
}

// NodeState represents the state of a single node
type NodeState struct {
	Name            string
	CPUUsage        string
	MemoryUsage     string
	CPUCapacity     string
	MemoryCapacity  string
	Status          string
	ContainerCount  int
	LastObservation time.Time
}

// ResourceState represents the state of resources in a namespace
type ResourceState struct {
	Pods        []PodState
	Services    []ServiceState
	Deployments []DeploymentState
}

// PodState represents the state of a pod
type PodState struct {
	Name         string
	Status       string
	Namespace    string
	Age          time.Duration
	RestartCount int32
}

// ServiceState represents the state of a service
type ServiceState struct {
	Name      string
	Type      string
	Namespace string
}

// DeploymentState represents the state of a deployment
type DeploymentState struct {
	Name      string
	Replicas  int32
	Namespace string
}

// Observation represents a learning observation
type Observation struct {
	Timestamp time.Time
	Action    string
	State     ClusterState
	Reward    float64
}

// Event represents a Kubernetes event
type Event struct {
	Type      string
	Reason    string
	Message   string
	Timestamp time.Time
	Object    string
	Namespace string
}

// Anomaly represents a detected anomaly
type Anomaly struct {
	Type        string
	Resource    string
	Namespace   string
	Severity    string
	Description string
	Timestamp   time.Time
	Value       float64
	Threshold   float64
}

// AnomalyHistory represents the history of anomalies
type AnomalyHistory struct {
	Anomalies []Anomaly
	Events    []Event
	LastSave  time.Time
}

// Threshold represents statistical thresholds for anomaly detection
type Threshold struct {
	Mean       float64
	StdDev     float64
	Min        float64
	Max        float64
	LastUpdate time.Time
}
