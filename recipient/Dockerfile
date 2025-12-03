# Multi-stage build for recipient service
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make gcc musl-dev

# Set working directory
WORKDIR /build

# Copy go mod files
COPY recipient/go.mod recipient/go.sum ./
COPY shared/go.mod ../shared/

# Download dependencies
RUN go mod download

# Copy source code
COPY recipient/ .
COPY shared/ ../shared

# Run go mod tidy to fix dependencies
RUN go mod tidy

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=1.0.0 -X main.BuildTime=$(date -u +%Y%m%d-%H%M%S)" \
    -o recipient cmd/main.go

# Final stage
FROM alpine:3.18

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 -S recipient && \
    adduser -u 1000 -S recipient -G recipient

# Set working directory
WORKDIR /app

# Create necessary directories with proper permissions
# Note: /app/logs will be overridden by volume mount, but we create it anyway
RUN mkdir -p /app/logs /tmp/mqtt-recipient-store && \
    chmod 755 /app && \
    chmod 755 /app/logs && \
    chown -R recipient:recipient /app /tmp/mqtt-recipient-store

# Create a script to fix permissions at runtime
RUN echo '#!/bin/sh' > /app/entrypoint.sh && \
    echo 'if [ -d "/app/logs" ]; then' >> /app/entrypoint.sh && \
    echo '  if [ ! -w "/app/logs" ]; then' >> /app/entrypoint.sh && \
    echo '    echo "Warning: /app/logs is not writable, trying to create temp log dir"' >> /app/entrypoint.sh && \
    echo '    mkdir -p /tmp/logs && export LOG_DIR=/tmp/logs' >> /app/entrypoint.sh && \
    echo '  fi' >> /app/entrypoint.sh && \
    echo 'fi' >> /app/entrypoint.sh && \
    echo 'exec "$@"' >> /app/entrypoint.sh && \
    chmod +x /app/entrypoint.sh && \
    chown recipient:recipient /app/entrypoint.sh

# Copy binary from builder
COPY --from=builder /build/recipient /app/recipient

# Copy configuration
COPY --chown=recipient:recipient recipient/config.docker.yaml /app/config.yaml

# Switch to non-root user
USER recipient

# Expose metrics port
EXPOSE 8081

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8081/health || exit 1

# Run the service with entrypoint wrapper
ENTRYPOINT ["/app/entrypoint.sh", "/app/recipient"]
CMD ["-config", "/app/config.yaml"]
