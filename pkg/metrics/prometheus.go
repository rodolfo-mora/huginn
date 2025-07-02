package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rodgon/valkyrie/pkg/anomaly"
	"github.com/rodgon/valkyrie/pkg/config"
	"github.com/rodgon/valkyrie/pkg/types"
)

// PrometheusExporter exposes anomaly detection metrics to Prometheus
type PrometheusExporter struct {
	// Configuration
	config *config.Config

	// Current metrics - Raw values (only if nodes enabled)
	nodeCPURaw    *prometheus.GaugeVec
	nodeMemoryRaw *prometheus.GaugeVec

	// Current metrics - Percentages (only if nodes enabled)
	nodeCPUUsage    *prometheus.GaugeVec
	nodeMemoryUsage *prometheus.GaugeVec

	// Pod metrics (only if pods enabled)
	podRestartCount *prometheus.GaugeVec

	// Node capacity metrics (only if nodes enabled)
	nodeCPUCapacity    *prometheus.GaugeVec
	nodeMemoryCapacity *prometheus.GaugeVec

	// Statistical measures - Node (only if nodes enabled)
	nodeCPUMean      *prometheus.GaugeVec
	nodeCPUStdDev    *prometheus.GaugeVec
	nodeCPUEWMA      *prometheus.GaugeVec
	nodeMemoryMean   *prometheus.GaugeVec
	nodeMemoryStdDev *prometheus.GaugeVec
	nodeMemoryEWMA   *prometheus.GaugeVec

	// Statistical measures - Pod (only if pods enabled)
	podRestartMean   *prometheus.GaugeVec
	podRestartStdDev *prometheus.GaugeVec
	podRestartEWMA   *prometheus.GaugeVec

	// Anomaly detection (always enabled)
	anomalyDetected *prometheus.CounterVec
	anomalySeverity *prometheus.GaugeVec

	// Historical data points (always enabled)
	metricHistory *prometheus.GaugeVec

	// Detector instance
	detector *anomaly.Detector
}

// NewPrometheusExporter creates a new Prometheus exporter
func NewPrometheusExporter(detector *anomaly.Detector, cfg *config.Config) *PrometheusExporter {
	exporter := &PrometheusExporter{
		detector: detector,
		config:   cfg,
	}

	// Always create anomaly detection metrics
	exporter.anomalyDetected = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "valkyrie_anomaly_detected_total",
			Help: "Total number of anomalies detected",
		},
		[]string{"type", "resource", "namespace", "severity"},
	)

	exporter.anomalySeverity = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "valkyrie_anomaly_severity_score",
			Help: "Severity score of detected anomalies (1=low, 2=medium, 3=high)",
		},
		[]string{"type", "resource", "namespace"},
	)

	exporter.metricHistory = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "valkyrie_metric_history",
			Help: "Historical metric values for analysis",
		},
		[]string{"resource_type", "resource_id", "metric_type"},
	)

	// Create node metrics only if nodes are enabled
	if exporter.isResourceEnabled("nodes") {
		exporter.createNodeMetrics()
	}

	// Create pod metrics only if pods are enabled
	if exporter.isResourceEnabled("pods") {
		exporter.createPodMetrics()
	}

	return exporter
}

// createNodeMetrics creates all node-related metrics
func (e *PrometheusExporter) createNodeMetrics() {
	// Current metrics - Raw values
	e.nodeCPURaw = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "valkyrie_node_cpu_raw",
			Help: "Raw CPU usage for each node",
		},
		[]string{"node"},
	)

	e.nodeMemoryRaw = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "valkyrie_node_memory_raw",
			Help: "Raw memory usage for each node",
		},
		[]string{"node"},
	)

	// Current metrics - Percentages
	e.nodeCPUUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "valkyrie_node_cpu_usage_percent",
			Help: "Current CPU usage percentage for each node",
		},
		[]string{"node"},
	)

	e.nodeMemoryUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "valkyrie_node_memory_usage_percent",
			Help: "Current memory usage percentage for each node",
		},
		[]string{"node"},
	)

	// Node capacity metrics
	e.nodeCPUCapacity = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "valkyrie_node_cpu_capacity",
			Help: "CPU capacity for each node",
		},
		[]string{"node"},
	)

	e.nodeMemoryCapacity = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "valkyrie_node_memory_capacity",
			Help: "Memory capacity for each node",
		},
		[]string{"node"},
	)

	// Statistical measures
	e.nodeCPUMean = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "valkyrie_node_cpu_mean_percent",
			Help: "Mean CPU usage percentage for each node",
		},
		[]string{"node"},
	)

	e.nodeCPUStdDev = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "valkyrie_node_cpu_stddev_percent",
			Help: "Standard deviation of CPU usage for each node",
		},
		[]string{"node"},
	)

	e.nodeCPUEWMA = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "valkyrie_node_cpu_ewma_percent",
			Help: "Exponentially weighted moving average of CPU usage for each node",
		},
		[]string{"node"},
	)

	e.nodeMemoryMean = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "valkyrie_node_memory_mean_percent",
			Help: "Mean memory usage percentage for each node",
		},
		[]string{"node"},
	)

	e.nodeMemoryStdDev = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "valkyrie_node_memory_stddev_percent",
			Help: "Standard deviation of memory usage for each node",
		},
		[]string{"node"},
	)

	e.nodeMemoryEWMA = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "valkyrie_node_memory_ewma_percent",
			Help: "Exponentially weighted moving average of memory usage for each node",
		},
		[]string{"node"},
	)
}

// createPodMetrics creates all pod-related metrics
func (e *PrometheusExporter) createPodMetrics() {
	e.podRestartCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "valkyrie_pod_restart_count",
			Help: "Current restart count for each pod",
		},
		[]string{"pod", "namespace"},
	)

	e.podRestartMean = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "valkyrie_pod_restart_mean",
			Help: "Mean restart count for each pod",
		},
		[]string{"pod", "namespace"},
	)

	e.podRestartStdDev = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "valkyrie_pod_restart_stddev",
			Help: "Standard deviation of restart count for each pod",
		},
		[]string{"pod", "namespace"},
	)

	e.podRestartEWMA = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "valkyrie_pod_restart_ewma",
			Help: "Exponentially weighted moving average of restart count for each pod",
		},
		[]string{"pod", "namespace"},
	)
}

// isResourceEnabled checks if a resource type is enabled in configuration
func (e *PrometheusExporter) isResourceEnabled(resourceType string) bool {
	// Check if any cluster has this resource enabled
	for _, cluster := range e.config.Clusters {
		if !cluster.Enabled {
			continue
		}
		for _, resource := range cluster.Resources {
			if resource == resourceType {
				return true
			}
		}
	}
	return false
}

// UpdateMetrics updates all Prometheus metrics based on current cluster state
func (e *PrometheusExporter) UpdateMetrics(state types.ClusterState) {
	// Reset all metrics to avoid stale data
	e.resetMetrics()

	// Update node metrics only if nodes are enabled
	if e.isResourceEnabled("nodes") && e.nodeCPUUsage != nil {
		for _, node := range state.Nodes {
			// Raw values (converted to numeric for Prometheus)
			cpuRaw := parseResourceValue(node.CPUUsage)
			memoryRaw := parseResourceValue(node.MemoryUsage)
			cpuCapacity := parseResourceValue(node.CPUCapacity)
			memoryCapacity := parseResourceValue(node.MemoryCapacity)

			// Set raw values
			if e.nodeCPURaw != nil {
				e.nodeCPURaw.WithLabelValues(node.Name).Set(cpuRaw)
			}
			if e.nodeMemoryRaw != nil {
				e.nodeMemoryRaw.WithLabelValues(node.Name).Set(memoryRaw)
			}

			// Set capacity values
			if e.nodeCPUCapacity != nil {
				e.nodeCPUCapacity.WithLabelValues(node.Name).Set(cpuCapacity)
			}
			if e.nodeMemoryCapacity != nil {
				e.nodeMemoryCapacity.WithLabelValues(node.Name).Set(memoryCapacity)
			}

			// Set percentage values (these are now correctly calculated)
			if e.nodeCPUUsage != nil {
				e.nodeCPUUsage.WithLabelValues(node.Name).Set(node.CPUUsagePercent)
			}
			if e.nodeMemoryUsage != nil {
				e.nodeMemoryUsage.WithLabelValues(node.Name).Set(node.MemoryUsagePercent)
			}

			// Update statistical measures for nodes
			e.updateNodeStats(node.Name, "cpu", node.CPUUsagePercent)
			e.updateNodeStats(node.Name, "memory", node.MemoryUsagePercent)
		}
	}

	// Update pod metrics only if pods are enabled
	if e.isResourceEnabled("pods") && e.podRestartCount != nil {
		for ns, resources := range state.Resources {
			for _, pod := range resources.Pods {
				restartCount := float64(pod.RestartCount)
				if e.podRestartCount != nil {
					e.podRestartCount.WithLabelValues(pod.Name, ns).Set(restartCount)
				}

				// Update statistical measures for pods
				e.updatePodStats(pod.Name, ns, restartCount)
			}
		}
	}

	// Update historical data points
	e.updateHistoricalMetrics()
}

// updateNodeStats updates statistical measures for a specific node and metric
func (e *PrometheusExporter) updateNodeStats(nodeName, metricType string, currentValue float64) {
	history := e.detector.GetMetricHistory("node", nodeName, metricType)
	if len(history) == 0 {
		return
	}

	mean, stddev, ewma := e.detector.ComputeStats(history, 0.3) // Using default alpha

	switch metricType {
	case "cpu":
		if e.nodeCPUMean != nil {
			e.nodeCPUMean.WithLabelValues(nodeName).Set(mean)
		}
		if e.nodeCPUStdDev != nil {
			e.nodeCPUStdDev.WithLabelValues(nodeName).Set(stddev)
		}
		if e.nodeCPUEWMA != nil {
			e.nodeCPUEWMA.WithLabelValues(nodeName).Set(ewma)
		}
	case "memory":
		if e.nodeMemoryMean != nil {
			e.nodeMemoryMean.WithLabelValues(nodeName).Set(mean)
		}
		if e.nodeMemoryStdDev != nil {
			e.nodeMemoryStdDev.WithLabelValues(nodeName).Set(stddev)
		}
		if e.nodeMemoryEWMA != nil {
			e.nodeMemoryEWMA.WithLabelValues(nodeName).Set(ewma)
		}
	}
}

// updatePodStats updates statistical measures for a specific pod
func (e *PrometheusExporter) updatePodStats(podName, namespace string, currentValue float64) {
	history := e.detector.GetMetricHistory("pod", podName, "restarts")
	if len(history) == 0 {
		return
	}

	mean, stddev, ewma := e.detector.ComputeStats(history, 0.1) // Using default alpha 0.3

	if e.podRestartMean != nil {
		e.podRestartMean.WithLabelValues(podName, namespace).Set(mean)
	}
	if e.podRestartStdDev != nil {
		e.podRestartStdDev.WithLabelValues(podName, namespace).Set(stddev)
	}
	if e.podRestartEWMA != nil {
		e.podRestartEWMA.WithLabelValues(podName, namespace).Set(ewma)
	}
}

// updateHistoricalMetrics exports historical data points
func (e *PrometheusExporter) updateHistoricalMetrics() {
	// This would need access to the detector's history
	// For now, we'll export the most recent values
	// In a full implementation, you might want to export a rolling window
}

// RecordAnomaly records a detected anomaly
func (e *PrometheusExporter) RecordAnomaly(anomaly types.Anomaly) {
	severityScore := getSeverityScore(anomaly.Severity)

	e.anomalyDetected.WithLabelValues(
		anomaly.Type,
		anomaly.Resource,
		anomaly.Namespace,
		anomaly.Severity,
	).Inc()

	e.anomalySeverity.WithLabelValues(
		anomaly.Type,
		anomaly.Resource,
		anomaly.Namespace,
	).Set(severityScore)
}

// resetMetrics resets all metrics to avoid stale data
func (e *PrometheusExporter) resetMetrics() {
	// Reset node metrics only if they exist
	if e.nodeCPURaw != nil {
		e.nodeCPURaw.Reset()
	}
	if e.nodeMemoryRaw != nil {
		e.nodeMemoryRaw.Reset()
	}
	if e.nodeCPUUsage != nil {
		e.nodeCPUUsage.Reset()
	}
	if e.nodeMemoryUsage != nil {
		e.nodeMemoryUsage.Reset()
	}
	if e.nodeCPUCapacity != nil {
		e.nodeCPUCapacity.Reset()
	}
	if e.nodeMemoryCapacity != nil {
		e.nodeMemoryCapacity.Reset()
	}

	// Reset pod metrics only if they exist
	if e.podRestartCount != nil {
		e.podRestartCount.Reset()
	}

	// Reset statistical measures only if they exist
	if e.nodeCPUMean != nil {
		e.nodeCPUMean.Reset()
	}
	if e.nodeCPUStdDev != nil {
		e.nodeCPUStdDev.Reset()
	}
	if e.nodeCPUEWMA != nil {
		e.nodeCPUEWMA.Reset()
	}
	if e.nodeMemoryMean != nil {
		e.nodeMemoryMean.Reset()
	}
	if e.nodeMemoryStdDev != nil {
		e.nodeMemoryStdDev.Reset()
	}
	if e.nodeMemoryEWMA != nil {
		e.nodeMemoryEWMA.Reset()
	}
	if e.podRestartMean != nil {
		e.podRestartMean.Reset()
	}
	if e.podRestartStdDev != nil {
		e.podRestartStdDev.Reset()
	}
	if e.podRestartEWMA != nil {
		e.podRestartEWMA.Reset()
	}

	// Always reset anomaly and history metrics (they're always created)
	e.anomalySeverity.Reset()
	e.metricHistory.Reset()
}

// getSeverityScore converts severity string to numeric score
func getSeverityScore(severity string) float64 {
	switch severity {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	default:
		return 0
	}
}

// parseResourceValue converts a resource string to a float64
func parseResourceValue(value string) float64 {
	var numeric float64
	_, err := fmt.Sscanf(value, "%f", &numeric)
	if err != nil {
		return 0
	}
	return numeric
}
