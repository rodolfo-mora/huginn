package anomaly

import (
	"fmt"
	"time"

	"github.com/rodgon/valkyrie/pkg/types"
)

// Detector implements anomaly detection for Kubernetes resources
type Detector struct {
	cpuThreshold    float64
	memoryThreshold float64
	podRestarts     int
	history         []types.Observation
	maxHistorySize  int
}

// NewDetector creates a new anomaly detector
func NewDetector() *Detector {
	return &Detector{
		cpuThreshold:    80.0,
		memoryThreshold: 80.0,
		podRestarts:     3,
		maxHistorySize:  1000,
	}
}

// SetThresholds sets the detection thresholds
func (d *Detector) SetThresholds(cpu, memory float64, podRestarts int) {
	d.cpuThreshold = cpu
	d.memoryThreshold = memory
	d.podRestarts = podRestarts
}

// SetMaxHistorySize sets the maximum history size
func (d *Detector) SetMaxHistorySize(size int) {
	d.maxHistorySize = size
}

// DetectAnomalies checks for anomalies in the current state
func (d *Detector) DetectAnomalies(state types.ClusterState) []types.Anomaly {
	var anomalies []types.Anomaly

	// Check node anomalies
	for _, node := range state.Nodes {
		// Check CPU usage
		if cpuUsage := parseResourceValue(node.CPUUsage); cpuUsage > 0 {
			if cpuUsage > d.cpuThreshold {
				anomalies = append(anomalies, types.Anomaly{
					Type:        "HighCPUUsage",
					Resource:    node.Name,
					Severity:    "High",
					Description: fmt.Sprintf("CPU usage is %.2f%% (threshold: %.2f%%)", cpuUsage, d.cpuThreshold),
					Value:       cpuUsage,
					Threshold:   d.cpuThreshold,
					Timestamp:   time.Now(),
				})
			}
		}

		// Check memory usage
		if memoryUsage := parseResourceValue(node.MemoryUsage); memoryUsage > 0 {
			if memoryUsage > d.memoryThreshold {
				anomalies = append(anomalies, types.Anomaly{
					Type:        "HighMemoryUsage",
					Resource:    node.Name,
					Severity:    "High",
					Description: fmt.Sprintf("Memory usage is %.2f%% (threshold: %.2f%%)", memoryUsage, d.memoryThreshold),
					Value:       memoryUsage,
					Threshold:   d.memoryThreshold,
					Timestamp:   time.Now(),
				})
			}
		}
	}

	// Check pod anomalies
	for ns, resources := range state.Resources {
		for _, pod := range resources.Pods {
			// Check pod restarts
			if pod.RestartCount > int32(d.podRestarts) {
				anomalies = append(anomalies, types.Anomaly{
					Type:        "HighPodRestarts",
					Resource:    pod.Name,
					Namespace:   ns,
					Severity:    "Medium",
					Description: fmt.Sprintf("Pod has restarted %d times (threshold: %d)", pod.RestartCount, d.podRestarts),
					Value:       float64(pod.RestartCount),
					Threshold:   float64(d.podRestarts),
					Timestamp:   time.Now(),
				})
			}

			// Check pod status
			if pod.Status != "Running" {
				anomalies = append(anomalies, types.Anomaly{
					Type:        "PodNotRunning",
					Resource:    pod.Name,
					Namespace:   ns,
					Severity:    "High",
					Description: fmt.Sprintf("Pod is in %s state", pod.Status),
					Timestamp:   time.Now(),
				})
			}
		}
	}

	return anomalies
}

// parseResourceValue converts a resource string to a float64
func parseResourceValue(value string) float64 {
	// Remove units and convert to float
	var numeric float64
	_, err := fmt.Sscanf(value, "%f", &numeric)
	if err != nil {
		return 0
	}
	return numeric
}
