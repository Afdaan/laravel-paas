# ===========================================================
# Backend Dockerfile
# ===========================================================
# Multi-stage build for Go backend
# ===========================================================

# Stage 1: Build
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install git for go mod
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

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
    docker-cli

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/main .

# Copy templates (needed for deployment)
COPY --from=builder /app/../docker/templates /app/docker/templates

# Create directories
RUN mkdir -p /app/storage/projects

# Non-root user
RUN adduser -D -g '' appuser
RUN chown -R appuser:appuser /app
USER appuser

EXPOSE 8080

CMD ["./main"]
