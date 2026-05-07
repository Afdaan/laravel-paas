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
FROM debian:bookworm-slim

LABEL maintainer="Laravel PaaS"
LABEL description="Backend API Service"

# Install dependencies and Docker CLI with Buildx
RUN apt-get update && apt-get install -y \
    ca-certificates \
    git \
    curl \
    tar \
    gnupg \
    lsb-release \
    && mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://download.docker.com/linux/debian/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null \
    && apt-get update && apt-get install -y \
    docker-ce-cli \
    docker-buildx-plugin \
    && rm -rf /var/lib/apt/lists/*

# Install Railpack CLI
RUN curl -sSL https://railpack.com/install.sh | sh

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/main .

# Create directories for runtime mounts
RUN mkdir -p /app/storage/projects /app/docker/templates

# Health check
HEALTHCHECK --interval=5s --timeout=3s --start-period=2s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

EXPOSE 8080

CMD ["./main"]
