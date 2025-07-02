package anomaly

import (
	"fmt"
	"math"
	"strings"
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
	debug           bool
	minStdDev       float64
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
func NewDetector(cpuThreshold, memoryThreshold float64, podRestarts int, maxHistorySize int, cpuAlpha, memoryAlpha, restartAlpha float64, debug bool, minStdDev float64) *Detector {
	return &Detector{
		cpuThreshold:    cpuThreshold,
		memoryThreshold: memoryThreshold,
		podRestarts:     podRestarts,
		maxHistorySize:  maxHistorySize,
		debug:           debug,
		minStdDev:       minStdDev,
		cpuStats: &MetricStats{
			alpha: cpuAlpha,
		},
		memoryStats: &MetricStats{
			alpha: memoryAlpha,
		},
		restartStats: &MetricStats{
			alpha: restartAlpha,
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
func isAnomalyHistory(value, mean, stddev, ewma, threshold, minStdDev float64) bool {
	// Require minimum standard deviation to avoid false positives from tiny variations

	// If we don't have enough data or stddev is too small, only check threshold
	if stddev < minStdDev {
		return value > threshold
	}

	// Calculate z-score
	zScore := math.Abs((value - mean) / stddev)

	// Check EWMA deviation with minimum threshold to avoid noise
	minEwmaDeviation := 5.0 // Minimum 5% deviation from EWMA for percentage values
	ewmaDeviation := math.Abs(value - ewma)

	// Anomaly conditions:
	// 1. Z-score > 3 (statistical outlier)
	// 2. Value > threshold (absolute threshold)
	// 3. EWMA deviation > max(2*stddev, minEwmaDeviation) (significant change from trend)
	return zScore > 3 || value > threshold || ewmaDeviation > math.Max(2*stddev, minEwmaDeviation)
}

// DetectAnomalies checks for anomalies in the current state using history-based stats
func (d *Detector) DetectAnomalies(state types.ClusterState) []types.Anomaly {
	var anomalies []types.Anomaly

	// For each node, record and analyze metrics
	for _, node := range state.Nodes {
		// Use pre-calculated percentage values instead of raw resource values
		cpuUsagePercent := node.CPUUsagePercent
		memoryUsagePercent := node.MemoryUsagePercent

		d.recordObservation("node", node.Name, "cpu", cpuUsagePercent)
		d.recordObservation("node", node.Name, "memory", memoryUsagePercent)

		// Build namespace information for anomaly descriptions
		namespacesInfo := ""
		if len(node.Namespaces) > 0 {
			namespacesInfo = fmt.Sprintf(" (namespaces: %s)", strings.Join(node.Namespaces, ", "))
		}

		cpuVals := d.GetMetricHistory("node", node.Name, "cpu")
		// Require minimum history for statistical analysis
		if len(cpuVals) < 5 {
			// With insufficient history, only check absolute threshold
			if cpuUsagePercent > d.cpuThreshold {
				anomalies = append(anomalies, types.Anomaly{
					Type:     "HighCPUUsage",
					Resource: node.Name,
					Severity: "High",
					Description: fmt.Sprintf("CPU usage is %.2f%% (insufficient history for statistical analysis)%s",
						cpuUsagePercent, namespacesInfo),
					Value:     cpuUsagePercent,
					Threshold: d.cpuThreshold,
					Timestamp: time.Now(),
				})
			}
			continue
		}

		cpuMean, cpuStd, cpuEwma := d.ComputeStats(cpuVals, d.cpuStats.alpha)
		if d.debug {
			d.DebugAnomalyCheck("node", node.Name, "cpu", cpuUsagePercent, d.cpuThreshold)
		}
		if isAnomalyHistory(cpuUsagePercent, cpuMean, cpuStd, cpuEwma, d.cpuThreshold, d.minStdDev) {
			anomalies = append(anomalies, types.Anomaly{
				Type:     "HighCPUUsage",
				Resource: node.Name,
				Severity: "High",
				Description: fmt.Sprintf("CPU usage is %.2f%% (mean: %.2f%%, stddev: %.2f%%)%s",
					cpuUsagePercent, cpuMean, cpuStd, namespacesInfo),
				Value:     cpuUsagePercent,
				Threshold: d.cpuThreshold,
				Timestamp: time.Now(),
			})
		}

		memoryVals := d.GetMetricHistory("node", node.Name, "memory")
		// Require minimum history for statistical analysis
		if len(memoryVals) < 5 {
			// With insufficient history, only check absolute threshold
			if memoryUsagePercent > d.memoryThreshold {
				anomalies = append(anomalies, types.Anomaly{
					Type:     "HighMemoryUsage",
					Resource: node.Name,
					Severity: "High",
					Description: fmt.Sprintf("Memory usage is %.2f%% (insufficient history for statistical analysis)%s",
						memoryUsagePercent, namespacesInfo),
					Value:     memoryUsagePercent,
					Threshold: d.memoryThreshold,
					Timestamp: time.Now(),
				})
			}
			continue
		}

		memMean, memStd, memEwma := d.ComputeStats(memoryVals, d.memoryStats.alpha)
		if d.debug {
			d.DebugAnomalyCheck("node", node.Name, "memory", memoryUsagePercent, d.memoryThreshold)
		}
		if isAnomalyHistory(memoryUsagePercent, memMean, memStd, memEwma, d.memoryThreshold, d.minStdDev) {
			anomalies = append(anomalies, types.Anomaly{
				Type:     "HighMemoryUsage",
				Resource: node.Name,
				Severity: "High",
				Description: fmt.Sprintf("Memory usage is %.2f%% (mean: %.2f%%, stddev: %.2f%%)%s",
					memoryUsagePercent, memMean, memStd, namespacesInfo),
				Value:     memoryUsagePercent,
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

			// Require minimum history for statistical analysis
			if len(restartVals) < 3 {
				// With insufficient history, only check absolute threshold
				if restartCount > float64(d.podRestarts) {
					anomalies = append(anomalies, types.Anomaly{
						Type:      "HighPodRestarts",
						Resource:  pod.Name,
						Namespace: ns,
						Severity:  "Medium",
						Description: fmt.Sprintf("Pod has restarted %d times (insufficient history for statistical analysis)",
							pod.RestartCount),
						Value:     restartCount,
						Threshold: float64(d.podRestarts),
						Timestamp: time.Now(),
					})
				}
				continue
			}

			rMean, rStd, rEwma := d.ComputeStats(restartVals, d.restartStats.alpha)
			if isAnomalyHistory(restartCount, rMean, rStd, rEwma, float64(d.podRestarts), d.minStdDev) {
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

	// Check for problematic events
	for _, event := range state.Events {
		// Check for error events
		if event.Severity == "Error" {
			anomalies = append(anomalies, types.Anomaly{
				Type:        "ClusterEvent",
				Resource:    event.Resource,
				Namespace:   event.Namespace,
				Severity:    "High",
				Description: fmt.Sprintf("Error event: %s - %s (count: %d)", event.Reason, event.Message, event.Count),
				Timestamp:   event.Timestamp,
			})
		}

		// Check for warning events with high count (indicating recurring issues)
		if event.Severity == "Warning" && event.Count > 5 {
			anomalies = append(anomalies, types.Anomaly{
				Type:        "ClusterEvent",
				Resource:    event.Resource,
				Namespace:   event.Namespace,
				Severity:    "Medium",
				Description: fmt.Sprintf("Recurring warning: %s - %s (count: %d)", event.Reason, event.Message, event.Count),
				Timestamp:   event.Timestamp,
			})
		}

		// Check for specific problematic event types
		problematicReasons := []string{
			"FailedScheduling",
			"FailedMount",
			"FailedAttachVolume",
			"FailedCreate",
			"FailedDelete",
			"BackOff",
			"CrashLoopBackOff",
			"ImagePullBackOff",
		}

		for _, reason := range problematicReasons {
			if event.Reason == reason {
				anomalies = append(anomalies, types.Anomaly{
					Type:        "ClusterEvent",
					Resource:    event.Resource,
					Namespace:   event.Namespace,
					Severity:    "High",
					Description: fmt.Sprintf("Problematic event: %s - %s (count: %d)", event.Reason, event.Message, event.Count),
					Timestamp:   event.Timestamp,
				})
				break
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

// DebugAnomalyCheck provides detailed information about anomaly detection decisions
func (d *Detector) DebugAnomalyCheck(resourceType, resourceID, metricType string, currentValue, threshold float64) {
	history := d.GetMetricHistory(resourceType, resourceID, metricType)

	fmt.Printf("=== Anomaly Debug for %s/%s %s ===\n", resourceType, resourceID, metricType)
	fmt.Printf("Current value: %.2f\n", currentValue)
	fmt.Printf("Threshold: %.2f\n", threshold)
	fmt.Printf("History length: %d\n", len(history))

	if len(history) < 5 {
		fmt.Printf("Insufficient history (< 5 observations), only checking threshold\n")
		return
	}

	mean, stddev, ewma := d.ComputeStats(history, d.getAlphaForMetric(metricType))
	fmt.Printf("Mean: %.2f\n", mean)
	fmt.Printf("StdDev: %.2f\n", stddev)
	fmt.Printf("EWMA: %.2f\n", ewma)

	zScore := math.Abs((currentValue - mean) / stddev)
	ewmaDeviation := math.Abs(currentValue - ewma)
	minEwmaDeviation := 5.0

	fmt.Printf("Z-Score: %.2f (threshold: 3.0)\n", zScore)
	fmt.Printf("EWMA Deviation: %.2f (threshold: %.2f)\n", ewmaDeviation, math.Max(2*stddev, minEwmaDeviation))
	fmt.Printf("StdDev check: %.2f >= %.2f = %t\n", stddev, d.minStdDev, stddev >= d.minStdDev)

	// Check each condition
	condition1 := zScore > 3
	condition2 := currentValue > threshold
	condition3 := ewmaDeviation > math.Max(2*stddev, minEwmaDeviation)

	fmt.Printf("Condition 1 (Z-Score > 3): %t\n", condition1)
	fmt.Printf("Condition 2 (Value > Threshold): %t\n", condition2)
	fmt.Printf("Condition 3 (EWMA Deviation): %t\n", condition3)
	fmt.Printf("Final result: %t\n", condition1 || condition2 || condition3)
	fmt.Printf("================================\n")
}

// getAlphaForMetric returns the appropriate alpha value for a metric type
func (d *Detector) getAlphaForMetric(metricType string) float64 {
	switch metricType {
	case "cpu":
		return d.cpuStats.alpha
	case "memory":
		return d.memoryStats.alpha
	case "restarts":
		return d.restartStats.alpha
	default:
		return 0.3
	}
}
