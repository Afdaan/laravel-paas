# ===========================================================
# Standalone Worker Dockerfile
# ===========================================================

# Stage 1: Build
FROM golang:1.22-alpine AS builder

WORKDIR /src

# Copy go workspace configuration first to allow dependency caching
COPY go.work go.work.sum ./
COPY shared/go.mod shared/go.sum ./shared/
COPY backend/go.mod backend/go.sum ./backend/
COPY worker/go.mod worker/go.sum ./worker/

# Download Go modules using BuildKit cache mount
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source code files
COPY shared/ ./shared/
COPY worker/ ./worker/

WORKDIR /src/worker
# Compile using BuildKit cache mounts for package modules and compiler cache
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/worker ./cmd/worker

# Stage 2: Production
FROM debian:bookworm-slim

LABEL maintainer="Laravel PaaS"
LABEL description="Standalone Worker Daemon"

ENV TZ=Asia/Jakarta

RUN apt-get update && apt-get install -y \
    ca-certificates \
    git \
    curl \
tar \
    gnupg \
    lsb-release \
    tzdata \
    && ln -fs /usr/share/zoneinfo/Asia/Jakarta /etc/localtime \
    && dpkg-reconfigure -f noninteractive tzdata \
    && mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://download.docker.com/linux/debian/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null \
    && apt-get update && apt-get install -y \
    docker-ce-cli \
    docker-buildx-plugin \
    && rm -rf /var/lib/apt/lists/*

RUN curl -sSL https://railpack.com/install.sh | sh

WORKDIR /app

COPY --from=builder /app/worker .

RUN mkdir -p /app/storage/projects /app/docker/templates

CMD ["./worker"]
