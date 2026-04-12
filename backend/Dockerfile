# ===========================================================
# Backend Dockerfile
# ===========================================================
# Multi-stage build for Go backend
# ===========================================================

# Stage 1: Build
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Enable Go Proxy for better connectivity in restricted networks
ENV GOPROXY=https://proxy.golang.org,direct

# Copy only dependency files first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Run tidy to ensure everything is clean
RUN go mod tidy

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/server

# Stage 2: Production
FROM alpine:latest

LABEL maintainer="Laravel PaaS"
LABEL description="Backend API Service"

# Install dependencies
RUN apk --no-cache add \
    ca-certificates \
    git \
    docker-cli \
    docker-cli-buildx \
    curl

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/main .

# Create directories for runtime mounts
RUN mkdir -p /app/storage/projects /app/docker/templates

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

EXPOSE 8080

CMD ["./main"]
