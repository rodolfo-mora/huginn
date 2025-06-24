package types

import (
	"time"
)

// ClusterState represents the current state of the Kubernetes cluster
type ClusterState struct {
	Namespaces []string
	Nodes      []Node
	Resources  map[string]ResourceList
}

// Node represents a Kubernetes node
type Node struct {
	Name            string
	CPUUsage        string
	MemoryUsage     string
	Condition       string
	ConditionStatus string
	Status          string
}

// ResourceList represents a list of resources in a namespace
type ResourceList struct {
	Pods        []Pod
	Services    []Service
	Deployments []Deployment
}

// Pod represents a Kubernetes pod
type Pod struct {
	Name         string
	Namespace    string
	Status       string
	RestartCount int32
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

// Anomaly represents a detected anomaly in the cluster
type Anomaly struct {
	Type        string
	Resource    string
	Namespace   string
	Severity    string
	Description string
	Value       float64
	Threshold   float64
	Timestamp   time.Time
	Labels      map[string]string
	Events      []Event
	Metadata    map[string]interface{}
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
	Timestamp time.Time
	State     ClusterState
	Reward    float64
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
	Alerts []AlertmanagerAlert // `json:"alerts"`
}
