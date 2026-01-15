#!/bin/bash
# ===========================================
# Laravel PaaS Stop Script
# ===========================================

set -e

echo "🛑 Stopping Laravel PaaS..."

# Stop containers (in reverse order)
docker stop paas-frontend 2>/dev/null || true
docker stop paas-backend 2>/dev/null || true
docker stop paas-traefik 2>/dev/null || true
docker stop paas-redis 2>/dev/null || true
docker stop paas-mysql 2>/dev/null || true

echo "✅ All containers stopped"

# Optionally remove containers
if [ "$1" == "--clean" ]; then
    echo "🗑️  Removing containers..."
    docker rm paas-frontend paas-backend paas-traefik paas-redis paas-mysql 2>/dev/null || true
    echo "✅ Containers removed"
fi

if [ "$1" == "--purge" ]; then
    echo "🗑️  Removing containers and volumes..."
    docker rm paas-frontend paas-backend paas-traefik paas-redis paas-mysql 2>/dev/null || true
    docker volume rm paas-mysql-data paas-redis-data paas-letsencrypt 2>/dev/null || true
    echo "✅ Containers and volumes removed"
fi
