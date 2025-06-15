# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o valkyrie ./main.go

# Final stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/valkyrie .

# Copy configuration and scripts
COPY config.yaml /app/config.yaml
COPY scripts/setup_qdrant.sh /app/scripts/
RUN chmod +x /app/scripts/setup_qdrant.sh

# Set environment variables
ENV TZ=UTC

# Run as non-root user
RUN adduser -D -g '' valkyrie
USER valkyrie

# Expose port if needed (adjust as necessary)
# EXPOSE 8080

# Set entrypoint
ENTRYPOINT ["/app/valkyrie"] 