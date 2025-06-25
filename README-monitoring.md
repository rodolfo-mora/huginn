# Valkyrie Monitoring Setup

This directory contains a complete monitoring stack for the Valkyrie anomaly detection system using Prometheus, Alertmanager, and Grafana.

## Quick Start

1. **Start the monitoring stack:**
   ```bash
   docker-compose up -d
   ```

2. **Start your Valkyrie application** (make sure it's running on localhost:8080):
   ```go
   agent.StartMetricsServer() // This starts the metrics server on :8080
   ```

3. **Access the monitoring interfaces:**
   - **Prometheus**: http://localhost:9190
   - **Alertmanager**: http://localhost:9093
   - **Grafana**: http://localhost:3000 (admin/admin)

## Configuration

### Prometheus Configuration (`prometheus.yml`)
- Scrapes metrics from `host.docker.internal:8080` (your Valkyrie app)
- 30-second scrape interval
- 200-hour data retention
- Integrated with Alertmanager for alerting

### Resource-Based Metrics
**Important**: Prometheus metrics are only created for resources enabled in your `kubernetes.resources` configuration:

- **If `nodes` is enabled**: All node metrics (CPU, memory, capacity, statistics) are created
- **If `pods` is enabled**: All pod metrics (restart counts, statistics) are created
- **If `services` is enabled**: Service metrics are created (future enhancement)
- **If `deployments` is enabled**: Deployment metrics are created (future enhancement)

**Always enabled metrics**:
- Anomaly detection metrics (`valkyrie_anomaly_detected_total`, `valkyrie_anomaly_severity_score`)
- Historical data metrics (`valkyrie_metric_history`)

This ensures efficient resource usage and prevents unnecessary metric collection.

### Alertmanager Configuration (`alertmanager.yml`)
- Webhook notifications (configurable endpoint)
- Email notifications (requires SMTP configuration)
- Alert grouping and inhibition rules
- 5-minute resolve timeout

### Alerting Rules (`valkyrie_alerts.yml`)
- **High/Critical CPU Usage**: >80% (warning), >90% (critical)
- **High/Critical Memory Usage**: >80% (warning), >90% (critical)
- **High/Critical Pod Restarts**: >5 (warning), >10 (critical)
- **Anomaly Detection**: Any anomaly detected
- **High Anomaly Rate**: >0.1 anomalies/second
- **Deviation from Mean**: >2x historical mean
- **Service Down**: Valkyrie metrics endpoint unavailable

### Grafana Configuration
- **Datasource**: Automatically configured to connect to Prometheus
- **Dashboard**: Pre-configured dashboard for Valkyrie metrics
- **Credentials**: admin/admin

## Available Metrics

### Node Metrics
- `valkyrie_node_cpu_raw` - Raw CPU usage per node (in cores, e.g., 1.5)
- `valkyrie_node_memory_raw` - Raw memory usage per node (in bytes)
- `valkyrie_node_cpu_capacity` - Total CPU capacity per node (in cores)
- `valkyrie_node_memory_capacity` - Total memory capacity per node (in bytes)
- `valkyrie_node_cpu_usage_percent` - Current CPU usage percentage per node (0-100)
- `valkyrie_node_memory_usage_percent` - Current memory usage percentage per node (0-100)
- `valkyrie_node_cpu_mean_percent` - Mean CPU usage percentage per node
- `valkyrie_node_cpu_stddev_percent` - Standard deviation of CPU usage per node
- `valkyrie_node_cpu_ewma_percent` - EWMA of CPU usage per node
- Similar metrics for memory

### Pod Metrics
- `valkyrie_pod_restart_count` - Current restart count per pod
- `valkyrie_pod_restart_mean` - Mean restart count per pod
- `valkyrie_pod_restart_stddev` - Standard deviation of restart count per pod
- `valkyrie_pod_restart_ewma` - EWMA of restart count per pod

### Anomaly Detection
- `valkyrie_anomaly_detected_total` - Counter of detected anomalies
- `valkyrie_anomaly_severity_score` - Severity score of anomalies

## Alerting

### Alert Severity Levels
- **Warning**: Issues that need attention but aren't critical
- **Critical**: Issues that require immediate attention

### Alert Types
1. **Resource Usage Alerts**
   - High CPU/Memory usage on nodes
   - Critical thresholds for immediate action

2. **Pod Health Alerts**
   - High restart counts indicating instability
   - Critical restart thresholds

3. **Anomaly Detection Alerts**
   - Any anomaly detected by Valkyrie
   - High anomaly rates indicating system issues

4. **Statistical Deviation Alerts**
   - Usage significantly above historical means
   - Indicates unusual behavior patterns

5. **Service Health Alerts**
   - Valkyrie metrics endpoint availability
   - Ensures monitoring system health

### Configuring Notifications

To configure email notifications, update `alertmanager.yml`:

```yaml
global:
  smtp_smarthost: 'your-smtp-server:587'
  smtp_from: 'alerts@yourcompany.com'
  smtp_auth_username: 'your-email@yourcompany.com'
  smtp_auth_password: 'your-password'

receivers:
  - name: 'email-notifications'
    email_configs:
      - to: 'your-team@yourcompany.com'
```

To configure Slack notifications:

```yaml
receivers:
  - name: 'slack-notifications'
    slack_configs:
      - api_url: 'https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK'
        channel: '#alerts'
        title: 'Valkyrie Alert'
        text: '{{ range .Alerts }}{{ .Annotations.summary }}{{ end }}'
```

## Example Prometheus Queries

```promql
# Get current CPU usage percentage for all nodes
valkyrie_node_cpu_usage_percent

# Get current memory usage percentage for all nodes
valkyrie_node_memory_usage_percent

# Get raw CPU usage in cores
valkyrie_node_cpu_raw

# Get raw memory usage in bytes
valkyrie_node_memory_raw

# Get node capacity information
valkyrie_node_cpu_capacity
valkyrie_node_memory_capacity

# Calculate actual usage vs capacity ratio
valkyrie_node_cpu_raw / valkyrie_node_cpu_capacity * 100

# Get anomalies detected in the last hour
increase(valkyrie_anomaly_detected_total[1h])

# Get nodes with high CPU usage (>80%)
valkyrie_node_cpu_usage_percent > 80

# Compare current vs mean CPU usage
valkyrie_node_cpu_usage_percent / valkyrie_node_cpu_mean_percent

# Get pods with high restart counts
valkyrie_pod_restart_count > 5

# Get active alerts
ALERTS{alertstate="firing"}

# Get nodes with memory usage above 90%
valkyrie_node_memory_usage_percent > 90

# Calculate memory usage in GB
valkyrie_node_memory_raw / 1024 / 1024 / 1024

# Calculate CPU usage in millicores
valkyrie_node_cpu_raw * 1000
```

## Dashboard Features

The pre-configured Grafana dashboards include:

### Valkyrie Comprehensive Monitoring Dashboard
A comprehensive dashboard (`valkyrie-comprehensive-dashboard.json`) that displays all metrics from the Prometheus exporter:

#### Node Metrics Section:
- **CPU Usage (%)** - Real-time CPU usage percentage graphs with thresholds
- **Memory Usage (%)** - Real-time memory usage percentage graphs with thresholds
- **CPU Raw Usage (cores)** - Raw CPU usage in cores (e.g., 1.5 cores)
- **Memory Raw Usage (GB)** - Raw memory usage converted to GB
- **CPU/Memory Capacity** - Static capacity information for each node
- **Usage vs Capacity Ratio** - Calculated ratios showing utilization
- **CPU/Memory Statistics** - Mean, EWMA, and Standard Deviation trends

#### Pod Metrics Section:
- **Pod Restart Count** - Table view of current restart counts with color coding
- **Pod Restart Statistics** - Historical trends of restart statistics

#### Anomaly Detection Section:
- **Anomalies Detected** - Count of anomalies in the last hour
- **Anomaly Severity Score** - Current severity scores (1=low, 2=medium, 3=high)
- **Anomaly Rate** - Rate of anomalies per second over time
- **Anomalies by Type** - Pie chart showing distribution by anomaly type

#### Interactive Features:
- **Node Filter** - Filter metrics by specific nodes
- **Namespace Filter** - Filter pod metrics by namespace
- **Auto-refresh** - Dashboard refreshes every 30 seconds
- **Threshold Alerts** - Color-coded thresholds for quick visual identification

### Valkyrie Basic Dashboard
A simpler dashboard (`valkyrie-dashboard.json`) with essential metrics for quick overview.

### Dashboard Configuration
- **Time Range**: Default 1 hour, adjustable
- **Refresh Rate**: 30 seconds
- **Theme**: Dark mode
- **Units**: Appropriate units for each metric type (%, GB, cores, etc.)

## Troubleshooting

### Prometheus can't scrape metrics
1. Ensure your Valkyrie application is running on localhost:8080
2. Check that the metrics server is started: `agent.StartMetricsServer()`
3. Verify the `/metrics` endpoint is accessible: `curl http://localhost:8080/metrics`

### Alertmanager not receiving alerts
1. Check Prometheus alerting configuration: http://localhost:9190/config
2. Verify Alertmanager is running: http://localhost:9093
3. Check alert rules: http://localhost:9190/rules

### Grafana can't connect to Prometheus
1. Ensure both containers are running: `docker-compose ps`
2. Check Prometheus is accessible: http://localhost:9190
3. Verify the datasource configuration in Grafana

### No metrics appearing
1. Check that your Valkyrie application is calling `DetectAnomalies()` regularly
2. Verify the Prometheus targets page: http://localhost:9190/targets
3. Check the Prometheus logs: `docker-compose logs prometheus`

### Alerts not firing
1. Check alert rules are loaded: http://localhost:9190/rules
2. Verify metrics are being collected
3. Check alert expressions in `valkyrie_alerts.yml`

## Stopping the Stack

```bash
docker-compose down
```

To remove all data:
```bash
docker-compose down -v
``` 