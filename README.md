# Valkyrie

Valkyrie is an intelligent Kubernetes monitoring agent that uses reinforcement learning to detect and respond to anomalies in your cluster.

## Features

- **Anomaly Detection**: Detects anomalies in CPU usage, memory usage, pod restarts, and pod status
- **Vector Storage**: Stores and searches similar anomalies using vector embeddings
- **Multiple Storage Backends**: Supports both Qdrant and Redis for vector storage
- **Embedding Models**: Supports multiple embedding models (Simple, OpenAI, Sentence Transformers, Ollama)
- **Notification System**: Supports multiple notification channels (Slack, Email, Webhook, Alertmanager)
- **Configurable Thresholds**: Customize detection thresholds and history size
- **Kubernetes Integration**: Monitors pods, deployments, services, and nodes

## Configuration

Valkyrie is configured using a YAML file. Here's an example configuration:

```yaml
# Kubernetes configuration
kubernetes:
  kubeconfig: ~/.kube/config
  context: ""
  namespace: ""
  resources:
    - pods
    - deployments
    - services
    - nodes

# Storage configuration
storage:
  type: qdrant  # or redis
  qdrant:
    url: http://localhost:6333
    collection: alerts
    vectorSize: 384
    distanceMetric: cosine
  redis:
    url: localhost:6379
    password: ""
    db: 0
    keyPrefix: "valkyrie:"

# Notification configuration
notification:
  enabled: true
  type: alertmanager  # or slack, email, webhook
  minSeverity: warning
  slack:
    webhookUrl: ""
    channel: "#alerts"
    username: "Valkyrie"
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
    labels:
      app: valkyrie
      severity: warning

# Anomaly detection configuration
anomalyDetection:
  cpuThreshold: 80.0
  memoryThreshold: 80.0
  podRestartThreshold: 3
  maxHistorySize: 1000
  cpuAlpha: 0.3      # EWMA smoothing factor for CPU (0.1-0.8)
  memoryAlpha: 0.3   # EWMA smoothing factor for Memory (0.1-0.8)
  restartAlpha: 0.3  # EWMA smoothing factor for Pod Restarts (0.1-0.8)

# Embedding configuration
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

## Installation

1. Clone the repository:
```bash
git clone https://github.com/rodgon/valkyrie.git
cd valkyrie
```

2. Install dependencies:
```bash
go mod download
```

3. Build the binary:
```bash
go build -o valkyrie
```

## Usage

1. Create a configuration file (e.g., `config.yaml`) with your settings.

2. Run Valkyrie:
```bash
./valkyrie -config config.yaml
```

## Development

### Prerequisites

- Go 1.21 or later
- Kubernetes cluster (or minikube for local development)
- Qdrant or Redis for vector storage
- (Optional) OpenAI API key for OpenAI embeddings
- (Optional) Ollama server for Ollama embeddings (see https://ollama.com/)

### Building

```bash
go build -o valkyrie
```

### Testing

```bash
go test ./...
```

## License

MIT License - see LICENSE file for details
