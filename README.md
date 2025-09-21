# Huginn

Huginn is an intelligent Kubernetes monitoring agent that uses reinforcement learning to detect and respond to anomalies in your clusters. It supports monitoring multiple Kubernetes clusters simultaneously.

## Authors

- **Rodolfo Gonzalez** - *Initial work and primary maintainer* - [@rodolfo-mora](https://github.com/rodolfo-mora)

## Features

- **Multi-Cluster Support**: Monitor multiple Kubernetes clusters from a single agent
- **Anomaly Detection**: Detects anomalies in CPU usage, memory usage, pod restarts, pod status, and cluster events
- **Event Collection**: Monitors Kubernetes cluster events for comprehensive health tracking
- **Vector Storage**: Stores and searches similar anomalies using vector embeddings
- **Multiple Storage Backends**: Supports both Qdrant and Redis for vector storage
- **Embedding Models**: Supports multiple embedding models (Simple, OpenAI, Sentence Transformers, Ollama)
- **Notification System**: Supports multiple notification channels (Slack, Email, Webhook, Alertmanager)
- **Configurable Thresholds**: Customize detection thresholds and history size
- **Kubernetes Integration**: Monitors pods, deployments, services, nodes, and events
- **Cluster Health Monitoring**: Tracks health status of individual clusters

## Configuration

Huginn is configured using a YAML file. Here's an example configuration for multiple clusters:

```yaml
# Multi-cluster configuration
clusters:
  # Production cluster
  - name: "production"
    id: "prod-cluster-1"
    labels:
      environment: "production"
      region: "us-west-2"
      team: "platform"
    kubeconfig: "/path/to/prod-kubeconfig"
    context: "prod-context"
    namespace: ""
    resources:
      - "nodes"
      - "events"
      - "pods"
      - "services"
    enabled: true

  # Staging cluster
  - name: "staging"
    id: "staging-cluster-1"
    labels:
      environment: "staging"
      region: "us-west-2"
      team: "platform"
    kubeconfig: "/path/to/staging-kubeconfig"
    context: "staging-context"
    namespace: ""
    resources:
      - "nodes"
      - "events"
      - "pods"
    enabled: true

  # Development cluster
  - name: "development"
    id: "dev-cluster-1"
    labels:
      environment: "development"
      region: "us-east-1"
      team: "platform"
    kubeconfig: "/path/to/dev-kubeconfig"
    context: "dev-context"
    namespace: ""
    resources:
      - "nodes"
      - "events"
    enabled: true

# Storage configuration (shared across all clusters)
storage:
  type: qdrant  # or redis
  storeAlerts: true
  qdrant:
    url: http://localhost:6333
    collection: huginn-anomalies
    vectorSize: 384
    distanceMetric: cosine
  redis:
    url: localhost:6379
    password: ""
    db: 0
    keyPrefix: "huginn:"

# Notification configuration (shared across all clusters)
notification:
  enabled: true
  type: alertmanager  # or slack, email, webhook
  minSeverity: warning
  slack:
    webhookUrl: ""
    channel: "#alerts"
    username: "Huginn"
  email:
    smtpHost: ""
    smtpPort: 587
    smtpUser: ""
    smtpPassword: ""
    from: ""
    to: []
  webhook:
    url: ""
    method: POST
    headers: {}
  alertmanager:
    url: http://localhost:9093
    defaultLabels:
      service: huginn
      component: anomaly-detection

# Anomaly detection configuration (applies to all clusters)
anomalyDetection:
  cpuThreshold: 80.0
  memoryThreshold: 80.0
  podRestartThreshold: 3
  maxHistorySize: 1000
  cpuAlpha: 0.3      # EWMA smoothing factor for CPU (0.1-0.8)
  memoryAlpha: 0.3   # EWMA smoothing factor for Memory (0.1-0.8)
  restartAlpha: 0.3  # EWMA smoothing factor for Pod Restarts (0.1-0.8)
  minStdDev: 1.0     # Minimum standard deviation for statistical analysis (0.5-5.0)

# Embedding configuration (shared across all clusters)
embedding:
  type: ollama  # or simple, openai, sentence-transformers
  dimension: 384
  openai:
    apiKey: ""
    model: text-embedding-ada-002
  sentenceTransformers:
    model: all-MiniLM-L6-v2
    device: cpu
  ollama:
    url: http://localhost:11434
    model: nomic-embed-text
```

### Cluster Configuration

Each cluster in the `clusters` array can have the following configuration:

- **name**: Human-readable name for the cluster
- **id**: Unique identifier for the cluster
- **labels**: Key-value pairs for categorizing clusters (environment, region, team, etc.)
- **kubeconfig**: Path to the kubeconfig file for this cluster
- **context**: Kubernetes context to use (empty for default)
- **namespace**: Specific namespace to monitor (empty for all namespaces)
- **resources**: List of resources to monitor (nodes, events, pods, services, deployments)
- **enabled**: Whether this cluster should be monitored

### Backward Compatibility

For single-cluster deployments, Huginn will automatically create a default cluster configuration if no clusters are specified in the config file.

## Installation

1. Clone the repository:
```bash
git clone https://github.com/rodolfo-mora/huginn.git
cd huginn
```

2. Install dependencies:
```bash
go mod download
```

3. Build the binary:
```bash
go build -o huginn
```

## Usage

1. Create a configuration file (e.g., `config.yaml`) with your cluster settings.

2. Run Huginn:
```bash
./huginn -config config.yaml
```

3. For debugging, you can print cluster state and anomalies:
```bash
./huginn -config config.yaml -print-state -print-anomalies
```

## Multi-Cluster Architecture

Huginn uses a multi-agent architecture where:

- **MultiClusterAgent**: Orchestrates multiple cluster agents
- **ClusterManager**: Manages cluster health and state
- **Individual Agents**: Handle monitoring for each cluster
- **Shared Components**: Storage, notifications, and anomaly detection are shared across clusters

Each cluster is monitored independently, but anomalies and observations are aggregated for comprehensive analysis.

## Development

### Prerequisites

- Go 1.21 or later
- Kubernetes clusters (or minikube for local development)
- Qdrant or Redis for vector storage
- (Optional) OpenAI API key for OpenAI embeddings
- (Optional) Ollama server for Ollama embeddings (see https://ollama.com/)

### Building

```bash
go build -o huginn
```

### Testing

```bash
go test ./...
```

### Contributors

We welcome contributions! If you'd like to contribute to Huginn, please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

MIT License - see LICENSE file for details
