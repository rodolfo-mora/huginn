package anomaly

import (
	"fmt"
	"math"
	"time"

	"github.com/rodgon/valkyrie/pkg/types"
)

// Detector implements anomaly detection for Kubernetes resources
type Detector struct {
	cpuThreshold    float64
	memoryThreshold float64
	podRestarts     int
	history         []MetricObservation
	maxHistorySize  int
	// Statistical measures
	cpuStats     *MetricStats
	memoryStats  *MetricStats
	restartStats *MetricStats
}

// MetricObservation holds a single metric sample for history-based analysis
type MetricObservation struct {
	Timestamp    time.Time
	ResourceType string // "node", "pod", "namespace"
	ResourceID   string // node name, pod name, etc.
	MetricType   string // "cpu", "memory", "restarts"
	Value        float64
}

// MetricStats holds statistical measures for a metric
type MetricStats struct {
	// mean       float64
	// stdDev     float64
	// ewma       float64 // Exponential Weighted Moving Average
	alpha float64 // EWMA smoothing factor
	// lastUpdate time.Time
}

// NewDetector creates a new anomaly detector
// EWMA alpha tuning guidelines:
//   - alpha controls how much weight is given to the most recent value in EWMA.
//   - alpha close to 1: EWMA reacts quickly to recent changes (less smoothing, more sensitive).
//   - alpha close to 0: EWMA reacts slowly, smoothing out short-term fluctuations (more smoothing, less sensitive).
//   - For noisy data, use a lower alpha (e.g., 0.1–0.3) to smooth out spikes.
//   - For rapid change detection, use a higher alpha (e.g., 0.5–0.8).
//   - Typical values: 0.2–0.4 for most monitoring scenarios.
//   - Try different values and plot the results to see what works best for your use case.
func NewDetector() *Detector {
	return &Detector{
		cpuThreshold:    80,
		memoryThreshold: 80,
		podRestarts:     3,
		maxHistorySize:  1000,
		cpuStats: &MetricStats{
			alpha: 0.015, // Smoothing factor
		},
		memoryStats: &MetricStats{
			alpha: 0.015,
		},
		restartStats: &MetricStats{
			alpha: 0.015,
		},
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

func (d *Detector) PrintHistory() {
	for _, obs := range d.history {
		fmt.Printf("%s/%s %s: %f\n", obs.ResourceType, obs.ResourceID, obs.MetricType, obs.Value)
	}
}

// updateStats updates statistical measures for a metric
// func (s *MetricStats) updateStats(value float64) {
// 	now := time.Now()
// 	if now.Sub(s.lastUpdate) < s.updateInterval {
// 		return
// 	}

// 	// Update EWMA
// 	if s.ewma == 0 {
// 		s.ewma = value
// 	} else {
// 		s.ewma = s.alpha*value + (1-s.alpha)*s.ewma
// 	}

// 	// Update mean and standard deviation
// 	// This is a simplified implementation. In production, you'd want to use
// 	// a more sophisticated algorithm like Welford's online algorithm
// 	s.mean = (s.mean + value) / 2
// 	s.stdDev = math.Sqrt(math.Pow(s.stdDev, 2) + math.Pow(value-s.mean, 2))
// 	s.lastUpdate = now
// }

// // isAnomaly checks if a value is anomalous based on statistical measures
// func (s *MetricStats) isAnomaly(value float64, threshold float64) bool {
// 	if s.stdDev == 0 {
// 		return value > threshold
// 	}

// 	// Calculate z-score
// 	zScore := math.Abs((value - s.mean) / s.stdDev)

// 	// Consider it anomalous if:
// 	// 1. Z-score is high (e.g., > 3)
// 	// 2. Value is above threshold
// 	// 3. Value deviates significantly from EWMA
// 	return zScore > 3 || value > threshold || math.Abs(value-s.ewma) > 2*s.stdDev
// }

// recordObservation records a single metric observation with resource context
func (d *Detector) recordObservation(resourceType, resourceID, metricType string, value float64) {
	obs := MetricObservation{
		Timestamp:    time.Now(),
		ResourceType: resourceType,
		ResourceID:   resourceID,
		MetricType:   metricType,
		Value:        value,
	}
	d.history = append(d.history, obs)
	if len(d.history) > d.maxHistorySize {
		d.history = d.history[len(d.history)-d.maxHistorySize:]
	}
}

// GetMetricHistory extracts a slice of float64 values for a specific resource and metric type
func (d *Detector) GetMetricHistory(resourceType, resourceID, metricType string) []float64 {
	values := make([]float64, 0, len(d.history))
	for _, obs := range d.history {
		if obs.ResourceType == resourceType && obs.ResourceID == resourceID && obs.MetricType == metricType {
			values = append(values, obs.Value)
		}
	}
	return values
}

// ComputeStats calculates mean, stddev, and ewma for a metric from history
func (d *Detector) ComputeStats(values []float64, alpha float64) (mean, stddev, ewma float64) {
	if len(values) == 0 {
		return 0, 0, 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean = sum / float64(len(values))
	var variance float64
	for _, v := range values {
		variance += (v - mean) * (v - mean)
	}
	stddev = 0
	if len(values) > 1 {
		stddev = (variance / float64(len(values)-1))
		stddev = math.Sqrt(stddev)
	}
	// EWMA
	ewma = values[0]
	for i := 1; i < len(values); i++ {
		ewma = alpha*values[i] + (1-alpha)*ewma
	}
	return mean, stddev, ewma
}

// isAnomalyHistory checks if a value is anomalous based on history-based stats
func isAnomalyHistory(value, mean, stddev, ewma, threshold float64) bool {
	if stddev == 0 {
		return value > threshold
	}
	zScore := math.Abs((value - mean) / stddev)
	return zScore > 3 || value > threshold || math.Abs(value-ewma) > 2*stddev
}

// DetectAnomalies checks for anomalies in the current state using history-based stats
func (d *Detector) DetectAnomalies(state types.ClusterState) []types.Anomaly {
	var anomalies []types.Anomaly

	// For each node, record and analyze metrics
	for _, node := range state.Nodes {
		cpuUsage := parseResourceValue(node.CPUUsage)
		memoryUsage := parseResourceValue(node.MemoryUsage)
		d.recordObservation("node", node.Name, "cpu", cpuUsage)
		d.recordObservation("node", node.Name, "memory", memoryUsage)

		cpuVals := d.GetMetricHistory("node", node.Name, "cpu")
		cpuMean, cpuStd, cpuEwma := d.ComputeStats(cpuVals, d.cpuStats.alpha)
		if isAnomalyHistory(cpuUsage, cpuMean, cpuStd, cpuEwma, d.cpuThreshold) {
			anomalies = append(anomalies, types.Anomaly{
				Type:     "HighCPUUsage",
				Resource: node.Name,
				Severity: "High",
				Description: fmt.Sprintf("CPU usage is %.2f%% (mean: %.2f%%, stddev: %.2f%%)",
					cpuUsage, cpuMean, cpuStd),
				Value:     cpuUsage,
				Threshold: d.cpuThreshold,
				Timestamp: time.Now(),
			})
		}

		memoryVals := d.GetMetricHistory("node", node.Name, "memory")
		memMean, memStd, memEwma := d.ComputeStats(memoryVals, d.memoryStats.alpha)
		if isAnomalyHistory(memoryUsage, memMean, memStd, memEwma, d.memoryThreshold) {
			anomalies = append(anomalies, types.Anomaly{
				Type:     "HighMemoryUsage",
				Resource: node.Name,
				Severity: "High",
				Description: fmt.Sprintf("Memory usage is %.2f%% (mean: %.2f%%, stddev: %.2f%%)",
					memoryUsage, memMean, memStd),
				Value:     memoryUsage,
				Threshold: d.memoryThreshold,
				Timestamp: time.Now(),
			})
		}
	}

	// For each pod, record and analyze restarts
	for ns, resources := range state.Resources {
		for _, pod := range resources.Pods {
			restartCount := float64(pod.RestartCount)
			d.recordObservation("pod", pod.Name, "restarts", restartCount)
			restartVals := d.GetMetricHistory("pod", pod.Name, "restarts")
			rMean, rStd, rEwma := d.ComputeStats(restartVals, d.restartStats.alpha)
			if isAnomalyHistory(restartCount, rMean, rStd, rEwma, float64(d.podRestarts)) {
				anomalies = append(anomalies, types.Anomaly{
					Type:      "HighPodRestarts",
					Resource:  pod.Name,
					Namespace: ns,
					Severity:  "Medium",
					Description: fmt.Sprintf("Pod has restarted %d times (mean: %.2f, stddev: %.2f)",
						pod.RestartCount, rMean, rStd),
					Value:     restartCount,
					Threshold: float64(d.podRestarts),
					Timestamp: time.Now(),
				})
			}
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
