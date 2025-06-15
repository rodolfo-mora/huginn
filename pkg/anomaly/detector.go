package anomaly

import (
	"fmt"
	"math"
	"time"

	"github.com/rodgon/valkyrie/pkg/types"
)

// Detector represents the anomaly detection system
type Detector struct {
	CPUThresholds       map[string]types.Threshold
	MemoryThresholds    map[string]types.Threshold
	PodRestartThreshold int32
	HistoricalData      []types.Anomaly
	MaxHistorySize      int
	LastSave            time.Time
}

// NewDetector creates a new anomaly detector
func NewDetector() *Detector {
	return &Detector{
		CPUThresholds:       make(map[string]types.Threshold),
		MemoryThresholds:    make(map[string]types.Threshold),
		PodRestartThreshold: 5,
		MaxHistorySize:      100,
	}
}

// UpdateThresholds updates the statistical thresholds based on historical data
func (d *Detector) UpdateThresholds(state types.ClusterState) {
	// Update CPU thresholds
	for _, node := range state.Nodes {
		cpuUsage := parseResourceValue(node.CPUUsage)
		if threshold, exists := d.CPUThresholds[node.Name]; exists {
			// Update existing threshold
			threshold = updateThreshold(threshold, cpuUsage)
			d.CPUThresholds[node.Name] = threshold
		} else {
			// Create new threshold
			d.CPUThresholds[node.Name] = types.Threshold{
				Mean:       cpuUsage,
				StdDev:     0,
				Min:        cpuUsage,
				Max:        cpuUsage,
				LastUpdate: time.Now(),
			}
		}
	}

	// Update Memory thresholds
	for _, node := range state.Nodes {
		memoryUsage := parseResourceValue(node.MemoryUsage)
		if threshold, exists := d.MemoryThresholds[node.Name]; exists {
			// Update existing threshold
			threshold = updateThreshold(threshold, memoryUsage)
			d.MemoryThresholds[node.Name] = threshold
		} else {
			// Create new threshold
			d.MemoryThresholds[node.Name] = types.Threshold{
				Mean:       memoryUsage,
				StdDev:     0,
				Min:        memoryUsage,
				Max:        memoryUsage,
				LastUpdate: time.Now(),
			}
		}
	}
}

// DetectAnomalies checks for anomalies in the current cluster state
func (d *Detector) DetectAnomalies(state types.ClusterState) []types.Anomaly {
	var anomalies []types.Anomaly

	// Update thresholds
	d.UpdateThresholds(state)

	// Check for CPU anomalies
	for _, node := range state.Nodes {
		cpuUsage := parseResourceValue(node.CPUUsage)
		if threshold, exists := d.CPUThresholds[node.Name]; exists {
			// Check if CPU usage is more than 2 standard deviations from the mean
			if math.Abs(cpuUsage-threshold.Mean) > 2*threshold.StdDev {
				anomalies = append(anomalies, types.Anomaly{
					Type:     "CPU",
					Resource: node.Name,
					Severity: "High",
					Description: fmt.Sprintf("CPU usage (%.2f) is significantly different from normal (%.2f ± %.2f)",
						cpuUsage, threshold.Mean, threshold.StdDev),
					Timestamp: time.Now(),
					Value:     cpuUsage,
					Threshold: threshold.Mean + 2*threshold.StdDev,
				})
			}
		}
	}

	// Check for Memory anomalies
	for _, node := range state.Nodes {
		memoryUsage := parseResourceValue(node.MemoryUsage)
		if threshold, exists := d.MemoryThresholds[node.Name]; exists {
			// Check if Memory usage is more than 2 standard deviations from the mean
			if math.Abs(memoryUsage-threshold.Mean) > 2*threshold.StdDev {
				anomalies = append(anomalies, types.Anomaly{
					Type:     "Memory",
					Resource: node.Name,
					Severity: "High",
					Description: fmt.Sprintf("Memory usage (%.2f) is significantly different from normal (%.2f ± %.2f)",
						memoryUsage, threshold.Mean, threshold.StdDev),
					Timestamp: time.Now(),
					Value:     memoryUsage,
					Threshold: threshold.Mean + 2*threshold.StdDev,
				})
			}
		}
	}

	// Check for pod anomalies
	for ns, resources := range state.Resources {
		for _, pod := range resources.Pods {
			// Check for frequently restarting pods
			if pod.RestartCount > d.PodRestartThreshold {
				anomalies = append(anomalies, types.Anomaly{
					Type:        "PodRestart",
					Resource:    pod.Name,
					Namespace:   ns,
					Severity:    "Medium",
					Description: fmt.Sprintf("Pod has restarted %d times", pod.RestartCount),
					Timestamp:   time.Now(),
					Value:       float64(pod.RestartCount),
					Threshold:   float64(d.PodRestartThreshold),
				})
			}

			// Check for pods in failed state
			if pod.Status == "Failed" {
				anomalies = append(anomalies, types.Anomaly{
					Type:        "PodStatus",
					Resource:    pod.Name,
					Namespace:   ns,
					Severity:    "High",
					Description: "Pod is in Failed state",
					Timestamp:   time.Now(),
				})
			}
		}
	}

	return anomalies
}

// updateThreshold updates a threshold with a new value
func updateThreshold(t types.Threshold, newValue float64) types.Threshold {
	// Update min/max
	if newValue < t.Min {
		t.Min = newValue
	}
	if newValue > t.Max {
		t.Max = newValue
	}

	// Update mean and standard deviation using Welford's online algorithm
	delta := newValue - t.Mean
	t.Mean += delta / float64(t.LastUpdate.Second()+1)
	delta2 := newValue - t.Mean
	t.StdDev += delta * delta2

	t.LastUpdate = time.Now()
	return t
}

// parseResourceValue converts a resource string (e.g., "100m", "1Gi") to a float64
func parseResourceValue(value string) float64 {
	var result float64
	var unit string
	fmt.Sscanf(value, "%f%s", &result, &unit)

	switch unit {
	case "m":
		return result / 1000
	case "Ki":
		return result * 1024
	case "Mi":
		return result * 1024 * 1024
	case "Gi":
		return result * 1024 * 1024 * 1024
	default:
		return result
	}
}
