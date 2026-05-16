#!/bin/bash
# ===========================================
# Laravel PaaS Stop Script
# ===========================================

set -e

echo "[STOP] Stopping Laravel PaaS..."

# Stop containers (in reverse order)
docker ps -a --format '{{.Names}}' | grep '^paas-worker-s' | xargs -r docker rm -f 2>/dev/null || true
docker stop paas-frontend 2>/dev/null || true
docker stop paas-backend 2>/dev/null || true
docker stop paas-traefik 2>/dev/null || true
docker stop paas-redis 2>/dev/null || true
docker stop paas-mysql 2>/dev/null || true
docker stop paas-postgres 2>/dev/null || true

echo "[SUCCESS] All containers stopped"

# Optionally remove containers
if [ "$1" == "--clean" ]; then
    echo "[CLEAN] Removing containers..."
    docker rm paas-frontend paas-backend paas-traefik paas-redis paas-mysql paas-postgres 2>/dev/null || true
    echo "[SUCCESS] Containers removed"
fi

if [ "$1" == "--purge" ]; then
    echo "[PURGE] Removing containers and volumes..."
    docker rm paas-frontend paas-backend paas-traefik paas-redis paas-mysql paas-postgres 2>/dev/null || true
    docker volume rm paas-redis-data paas-letsencrypt 2>/dev/null || true
    echo "[SUCCESS] Containers removed; Redis and TLS volumes purged (MySQL & PG data preserved)"
fi
