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

# Stage 2: Production (Lightweight Alpine ~25MB)
FROM alpine:3.19

LABEL maintainer="Laravel PaaS"
LABEL description="Backend API Service & Watchdog"

ENV TZ=Asia/Jakarta

# Install minimal runtime dependencies (docker-cli for WorkerManager orchestration)
RUN apk update && apk add --no-cache \
    ca-certificates \
    curl \
    docker-cli \
    tzdata && \
    cp /usr/share/zoneinfo/Asia/Jakarta /etc/localtime && \
    echo "Asia/Jakarta" > /etc/timezone

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/main .

# Health check
HEALTHCHECK --interval=5s --timeout=3s --start-period=2s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

EXPOSE 8080

CMD ["./main"]
