# ===========================================================
# Backend Dockerfile (Isolated API Runtime)
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
COPY backend/ ./backend/

WORKDIR /src/backend
# Compile using BuildKit cache mounts for package modules and compiler cache
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/main ./cmd/server

# Stage 2: Production
FROM alpine:3.19

LABEL maintainer="Runara"
LABEL description="Backend API Service"

ENV TZ=Asia/Jakarta

# Install minimal runtime dependencies (docker-cli for logs/stats)
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
