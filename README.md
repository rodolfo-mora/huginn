# Valkyrie - Kubernetes Anomaly Detection Agent

Valkyrie is a learning agent that monitors Kubernetes clusters for anomalies and provides intelligent insights into cluster health and performance.

## Features

- Real-time cluster state monitoring
- Statistical anomaly detection
- Event correlation and enrichment
- Vector-based alert storage with Qdrant or Redis
- Multiple notification channels (Alertmanager, Slack, Email, Webhook)
- Learning capabilities for adaptive thresholds

## Project Structure

```
.
├── main.go                 # Main entry point
├── pkg/
│   ├── agent/             # Core agent implementation
│   │   └── agent.go
│   ├── anomaly/           # Anomaly detection logic
│   │   └── detector.go
│   ├── embedding/         # Text embedding models
│   │   └── model.go
│   ├── notification/      # Notification system
│   │   ├── types.go
│   │   └── notifiers.go
│   ├── storage/          # Vector storage implementations
│   │   ├── storage.go    # Storage interface
│   │   ├── factory.go    # Storage factory
│   │   ├── qdrant.go     # Qdrant implementation
│   │   └── redis.go      # Redis implementation
│   └── types/            # Common types
│       └── types.go
└── scripts/              # Utility scripts
```

## Setup

1. Install dependencies:
   ```bash
   go mod download
   ```

2. Configure Kubernetes access:
   - Set `KUBECONFIG` environment variable or use default `~/.kube/config`

3. Configure storage backend:
   - Set `STORAGE_TYPE` to either "qdrant" or "redis" (default: "qdrant")
   
   For Qdrant:
   - Set `QDRANT_URL` (default: http://localhost:6333)
   - Run the setup script: `./scripts/setup_qdrant.sh`
   
   For Redis:
   - Set `REDIS_URL` (default: localhost:6379)
   - Set `REDIS_PASSWORD` (optional)
   - Set `REDIS_DB` (default: 0)

4. Configure notification channels (optional):
   - Alertmanager: Set `ALERTMANAGER_URL` environment variable
   - Slack: Set `SLACK_WEBHOOK_URL` and `SLACK_CHANNEL`
   - Email: Configure SMTP settings
   - Webhook: Set `WEBHOOK_ENDPOINT`

5. Run the agent:
   ```bash
   go run main.go
   ```

## Configuration

The agent can be configured through environment variables:

- `KUBECONFIG`: Path to kubeconfig file
- `STORAGE_TYPE`: Storage backend type (qdrant, redis)
- `QDRANT_URL`: Qdrant endpoint (default: http://localhost:6333)
- `REDIS_URL`: Redis endpoint (default: localhost:6379)
- `REDIS_PASSWORD`: Redis password (optional)
- `REDIS_DB`: Redis database number (default: 0)
- `ALERTMANAGER_URL`: Alertmanager endpoint (default: http://localhost:9093)
- `NOTIFICATION_TYPE`: Type of notification (alertmanager, slack, email, webhook)
- `MIN_SEVERITY`: Minimum severity for notifications (Low, Medium, High)

## Development

1. Install development tools:
   ```bash
   go install golang.org/x/tools/cmd/goimports@latest
   go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
   ```

2. Run tests:
   ```bash
   go test ./...
   ```

3. Run linter:
   ```bash
   golangci-lint run
   ```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Commit your changes
4. Push to the branch
5. Create a Pull Request

## License

MIT License
