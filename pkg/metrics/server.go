package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsServer provides an HTTP server to expose Prometheus metrics
type MetricsServer struct {
	addr     string
	exporter *PrometheusExporter
}

// NewMetricsServer creates a new metrics server
func NewMetricsServer(addr string, exporter *PrometheusExporter) *MetricsServer {
	return &MetricsServer{
		addr:     addr,
		exporter: exporter,
	}
}

// Start starts the metrics server
func (s *MetricsServer) Start() error {
	// Register the Prometheus handler
	http.Handle("/metrics", promhttp.Handler())

	// Start the server
	return http.ListenAndServe(s.addr, nil)
}

// StartAsync starts the metrics server in a goroutine
func (s *MetricsServer) StartAsync() {
	go func() {
		if err := s.Start(); err != nil {
			panic(err)
		}
	}()
}
