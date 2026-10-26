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

LABEL maintainer="Runara"
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

ARG RAILPACK_VERSION=0.43.0
RUN set -eu; \
    case "$(dpkg --print-architecture)" in \
        amd64) arch=x86_64; checksum=248a8f60f36854aabed6124113631078a802de3461660855fc4de9c26065c1fb ;; \
        arm64) arch=arm64; checksum=732ece14eb4f4676a43ebba19431f954edf7b130acb76159ef0f2ff86ac42ac9 ;; \
        *) echo "Unsupported Railpack architecture" >&2; exit 1 ;; \
    esac; \
    curl -fsSL --retry 3 "https://github.com/railwayapp/railpack/releases/download/v${RAILPACK_VERSION}/railpack-v${RAILPACK_VERSION}-${arch}-unknown-linux-musl.tar.gz" -o /tmp/railpack.tar.gz; \
    printf '%s  /tmp/railpack.tar.gz\n' "$checksum" | sha256sum -c -; \
    tar -xzf /tmp/railpack.tar.gz -C /usr/local/bin railpack; \
    rm /tmp/railpack.tar.gz; \
    railpack --version

WORKDIR /app

COPY --from=builder /app/worker .

RUN mkdir -p /app/storage/projects /app/docker/templates /app/railpacks

CMD ["./worker"]
