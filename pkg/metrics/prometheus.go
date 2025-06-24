package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rodgon/valkyrie/pkg/anomaly"
	"github.com/rodgon/valkyrie/pkg/types"
)

// PrometheusExporter exposes anomaly detection metrics to Prometheus
type PrometheusExporter struct {
	// Current metrics
	nodeCPUUsage    *prometheus.GaugeVec
	nodeMemoryUsage *prometheus.GaugeVec
	podRestartCount *prometheus.GaugeVec

	// Statistical measures
	nodeCPUMean      *prometheus.GaugeVec
	nodeCPUStdDev    *prometheus.GaugeVec
	nodeCPUEWMA      *prometheus.GaugeVec
	nodeMemoryMean   *prometheus.GaugeVec
	nodeMemoryStdDev *prometheus.GaugeVec
	nodeMemoryEWMA   *prometheus.GaugeVec
	podRestartMean   *prometheus.GaugeVec
	podRestartStdDev *prometheus.GaugeVec
	podRestartEWMA   *prometheus.GaugeVec

	// Anomaly detection
	anomalyDetected *prometheus.CounterVec
	anomalySeverity *prometheus.GaugeVec

	// Historical data points
	metricHistory *prometheus.GaugeVec

	// Detector instance
	detector *anomaly.Detector
}

// NewPrometheusExporter creates a new Prometheus exporter
func NewPrometheusExporter(detector *anomaly.Detector) *PrometheusExporter {
	return &PrometheusExporter{
		detector: detector,

		// Current metrics
		nodeCPUUsage: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "valkyrie_node_cpu_usage_percent",
				Help: "Current CPU usage percentage for each node",
			},
			[]string{"node"},
		),
		nodeMemoryUsage: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "valkyrie_node_memory_usage_percent",
				Help: "Current memory usage percentage for each node",
			},
			[]string{"node"},
		),
		podRestartCount: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "valkyrie_pod_restart_count",
				Help: "Current restart count for each pod",
			},
			[]string{"pod", "namespace"},
		),

		// Statistical measures
		nodeCPUMean: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "valkyrie_node_cpu_mean_percent",
				Help: "Mean CPU usage percentage for each node",
			},
			[]string{"node"},
		),
		nodeCPUStdDev: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "valkyrie_node_cpu_stddev_percent",
				Help: "Standard deviation of CPU usage for each node",
			},
			[]string{"node"},
		),
		nodeCPUEWMA: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "valkyrie_node_cpu_ewma_percent",
				Help: "Exponentially weighted moving average of CPU usage for each node",
			},
			[]string{"node"},
		),
		nodeMemoryMean: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "valkyrie_node_memory_mean_percent",
				Help: "Mean memory usage percentage for each node",
			},
			[]string{"node"},
		),
		nodeMemoryStdDev: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "valkyrie_node_memory_stddev_percent",
				Help: "Standard deviation of memory usage for each node",
			},
			[]string{"node"},
		),
		nodeMemoryEWMA: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "valkyrie_node_memory_ewma_percent",
				Help: "Exponentially weighted moving average of memory usage for each node",
			},
			[]string{"node"},
		),
		podRestartMean: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "valkyrie_pod_restart_mean",
				Help: "Mean restart count for each pod",
			},
			[]string{"pod", "namespace"},
		),
		podRestartStdDev: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "valkyrie_pod_restart_stddev",
				Help: "Standard deviation of restart count for each pod",
			},
			[]string{"pod", "namespace"},
		),
		podRestartEWMA: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "valkyrie_pod_restart_ewma",
				Help: "Exponentially weighted moving average of restart count for each pod",
			},
			[]string{"pod", "namespace"},
		),

		// Anomaly detection
		anomalyDetected: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "valkyrie_anomaly_detected_total",
				Help: "Total number of anomalies detected",
			},
			[]string{"type", "resource", "namespace", "severity"},
		),
		anomalySeverity: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "valkyrie_anomaly_severity_score",
				Help: "Severity score of detected anomalies (1=low, 2=medium, 3=high)",
			},
			[]string{"type", "resource", "namespace"},
		),

		// Historical data points
		metricHistory: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "valkyrie_metric_history",
				Help: "Historical metric values for analysis",
			},
			[]string{"resource_type", "resource_id", "metric_type"},
		),
	}
}

// UpdateMetrics updates all Prometheus metrics based on current cluster state
func (e *PrometheusExporter) UpdateMetrics(state types.ClusterState) {
	// Reset all metrics to avoid stale data
	e.resetMetrics()

	// Update current node metrics
	for _, node := range state.Nodes {
		cpuUsage := parseResourceValue(node.CPUUsage)
		memoryUsage := parseResourceValue(node.MemoryUsage)

		e.nodeCPUUsage.WithLabelValues(node.Name).Set(cpuUsage)
		e.nodeMemoryUsage.WithLabelValues(node.Name).Set(memoryUsage)

		// Update statistical measures for nodes
		e.updateNodeStats(node.Name, "cpu", cpuUsage)
		e.updateNodeStats(node.Name, "memory", memoryUsage)
	}

	// Update current pod metrics
	// for ns, resources := range state.Resources {
	// 	for _, pod := range resources.Pods {
	// 		restartCount := float64(pod.RestartCount)
	// 		e.podRestartCount.WithLabelValues(pod.Name, ns).Set(restartCount)

	// 		// Update statistical measures for pods
	// 		e.updatePodStats(pod.Name, ns, restartCount)
	// 	}
	// }

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
		e.nodeCPUMean.WithLabelValues(nodeName).Set(mean)
		e.nodeCPUStdDev.WithLabelValues(nodeName).Set(stddev)
		e.nodeCPUEWMA.WithLabelValues(nodeName).Set(ewma)
	case "memory":
		e.nodeMemoryMean.WithLabelValues(nodeName).Set(mean)
		e.nodeMemoryStdDev.WithLabelValues(nodeName).Set(stddev)
		e.nodeMemoryEWMA.WithLabelValues(nodeName).Set(ewma)
	}
}

// updatePodStats updates statistical measures for a specific pod
func (e *PrometheusExporter) updatePodStats(podName, namespace string, currentValue float64) {
	history := e.detector.GetMetricHistory("pod", podName, "restarts")
	if len(history) == 0 {
		return
	}

	mean, stddev, ewma := e.detector.ComputeStats(history, 0.1) // Using default alpha 0.3

	e.podRestartMean.WithLabelValues(podName, namespace).Set(mean)
	e.podRestartStdDev.WithLabelValues(podName, namespace).Set(stddev)
	e.podRestartEWMA.WithLabelValues(podName, namespace).Set(ewma)
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
	e.nodeCPUUsage.Reset()
	e.nodeMemoryUsage.Reset()
	e.podRestartCount.Reset()
	e.nodeCPUMean.Reset()
	e.nodeCPUStdDev.Reset()
	e.nodeCPUEWMA.Reset()
	e.nodeMemoryMean.Reset()
	e.nodeMemoryStdDev.Reset()
	e.nodeMemoryEWMA.Reset()
	e.podRestartMean.Reset()
	e.podRestartStdDev.Reset()
	e.podRestartEWMA.Reset()
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
