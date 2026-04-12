#!/bin/bash
set -e

# ==============================================================================
# Laravel PaaS CI/CD Deployment Script
# Only builds and deploys the specified application components (frontend/backend)
# Usage: ./deploy-app.sh [target]
# Targets:
#   backend  - Build & deploy backend only
#   frontend - Build & deploy frontend only
#   all      - Build & deploy both
# ==============================================================================

TARGET=${1:-"all"}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}[INFO] Starting Zero-Downtime Deployment for: ${TARGET^^}${NC}"

cd "$PROJECT_ROOT"

# Load .env if not already set
if [ -z "$MYSQL_ROOT_PASSWORD" ] && [ -f "$PROJECT_ROOT/.env" ]; then
    echo -e "${YELLOW}Loading .env file...${NC}"
    set -a
    source "$PROJECT_ROOT/.env"
    set +a
fi

# Set defaults matching start.sh
MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PASSWORD:-"rootpassword"}
MYSQL_USER=${MYSQL_USER:-"root"}
MYSQL_DATABASE=${MYSQL_DATABASE:-"paas"}
BASE_DOMAIN=${BASE_DOMAIN:-"localhost"}
JWT_SECRET=${JWT_SECRET:-"change-me-please-12345"}
MYSQL_PASSWORD=${MYSQL_PASSWORD:-"$MYSQL_ROOT_PASSWORD"}
PG_PASSWORD=${PG_PASSWORD:-"pgrootpassword"}
PG_USER=${PG_USER:-"postgres"}
PG_DATABASE=${PG_DATABASE:-"paas"}

# Smart Path Detection
if [[ "$PROJECTS_PATH" == "/app/storage/"* ]]; then
    PROJECTS_PATH="${PROJECT_ROOT}/${PROJECTS_PATH#/app/}"
fi
if [[ "$DATA_PATH" == "/app/storage/"* ]]; then
    DATA_PATH="${PROJECT_ROOT}/${DATA_PATH#/app/}"
fi

PROJECTS_PATH="${PROJECTS_PATH:-${PROJECT_ROOT}/storage/projects}"
DATA_PATH="${DATA_PATH:-${PROJECT_ROOT}/storage/data}"
HOST_DATA_PATH="${HOST_DATA_PATH:-$DATA_PATH}"
HOST_PROJECTS_PATH="${HOST_PROJECTS_PATH:-$PROJECTS_PATH}"

# Ensure directories exist and have correct permissions
sudo mkdir -p "$PROJECTS_PATH" "$DATA_PATH"
sudo chown -R $(id -u):$(id -g) "$PROJECTS_PATH" "$DATA_PATH"
chmod 777 "$DATA_PATH"

# Helper to get next numeric tag for a service
get_next_service_tag() {
    local service=$1
    local last_tag=$(docker images "paas-$service" --format "{{.Tag}}" | grep -E '^[0-9]+$' | sort -V | tail -n 1)
    if [ -z "$last_tag" ]; then
        echo "1"
    else
        echo $((last_tag + 1))
    fi
}

# Function to deploy with zero downtime
deploy_with_anti_downtime() {
    local service_name=$1
    local context_dir=$2
    local image_tag=$3
    shift 3
    
    local image_name="paas-$service_name:$image_tag"
    local container_name="paas-$service_name"
    local temp_container_name="${container_name}-new"
    local old_container_name="${container_name}-old"

    echo -e "${YELLOW}[DEPLOY] Working on $service_name (Tag: $image_tag)...${NC}"

    if ! DOCKER_BUILDKIT=1 docker build -t "$image_name" "$context_dir"; then
        echo -e "${RED}[ERROR] Build failed for $service_name. Keeping current version running.${NC}"
        return 1
    fi
    echo -e "${GREEN}[SUCCESS] Build complete: $image_name${NC}"

    docker rm -f "$temp_container_name" 2>/dev/null || true
    echo -e "${YELLOW}[RUN] Starting new container $temp_container_name...${NC}"
    
    if ! docker run -d --name "$temp_container_name" "$@" "$image_name"; then
        echo -e "${RED}[ERROR] Failed to start new container. Keeping current version.${NC}"
        return 1
    fi

    echo -e "${YELLOW}[HEALTH] Waiting for $service_name to be healthy...${NC}"
    local healthy=false
    for i in {1..15}; do
        local status=$(docker inspect --format='{{json .State.Health.Status}}' "$temp_container_name" 2>/dev/null | tr -d '"')
        if [ "$status" == "healthy" ]; then
            healthy=true
            break
        elif [ "$status" == "unhealthy" ]; then
            break
        fi
        if [ "$status" == "" ] || [ "$status" == "null" ]; then
             if [ "$(docker inspect -f '{{.State.Running}}' "$temp_container_name" 2>/dev/null)" == "true" ]; then
                if [ $i -ge 5 ]; then healthy=true; break; fi
             fi
        fi
        sleep 5
    done

    if [ "$healthy" == "true" ]; then
        echo -e "${GREEN}[SUCCESS] $service_name is healthy! Swapping containers...${NC}"
        
        # 1. Prepare for cutover by moving the current container to 'old' identity
        if docker ps -a --format '{{.Names}}' | grep -q "^${container_name}$"; then
            echo -e "${YELLOW}[SWAP] Reassigning current $container_name to $old_container_name...${NC}"
            docker rename "$container_name" "$old_container_name" 2>/dev/null || true
        fi
        
        # 2. Immediately assign the new container to the main identity
        echo -e "${YELLOW}[SWAP] Promoting $temp_container_name to $container_name...${NC}"
        docker rename "$temp_container_name" "$container_name"
        
        # 3. NOW stop the old version
        if docker ps -a --format '{{.Names}}' | grep -q "^${old_container_name}$"; then
            echo -e "${YELLOW}[STOP] Stopping previous $service_name version...${NC}"
            docker stop "$old_container_name" 2>/dev/null || true
            docker rm -f "$old_container_name" 2>/dev/null || true
        fi
        
        echo -e "${YELLOW}[CLEANUP] Removing old images for $service_name...${NC}"
        docker images "paas-$service_name" --format "{{.Tag}}" | grep -v "$image_tag" | xargs -I {} docker rmi "paas-$service_name:{}" 2>/dev/null || true
        
        return 0
    else
        echo -e "${RED}[ERROR] $service_name validation failed. Rolling back...${NC}"
        docker stop "$temp_container_name" 2>/dev/null || true
        docker rm -f "$temp_container_name" 2>/dev/null || true
        return 1
    fi
}

deploy_backend() {
    echo -e "${YELLOW}Deploying backend module...${NC}"
    BACKEND_TAG=$(get_next_service_tag "backend")
    
    deploy_with_anti_downtime "backend" "${PROJECT_ROOT}/backend" "$BACKEND_TAG" \
        --network paas-network \
        --restart unless-stopped \
        -v /var/run/docker.sock:/var/run/docker.sock \
        -v "${PROJECT_ROOT}/.env:/app/.env:ro" \
        -v "$PROJECTS_PATH:/app/storage/projects" \
        -v "$DATA_PATH:/app/storage/data" \
        -v "${PROJECT_ROOT}/docker/templates:/app/docker/templates:ro" \
        -e DATA_PATH="/app/storage/data" \
        -e PROJECTS_PATH="/app/storage/projects" \
        -e HOST_DATA_PATH="$HOST_DATA_PATH" \
        -e HOST_PROJECTS_PATH="$HOST_PROJECTS_PATH" \
        -e PG_HOST=paas-postgres \
        -e PG_USER="$PG_USER" \
        -e PG_PASSWORD="$PG_PASSWORD" \
        -e PG_DATABASE="$PG_DATABASE" \
        -e MYSQL_HOST=paas-mysql \
        -e MYSQL_ROOT_PASSWORD="$MYSQL_ROOT_PASSWORD" \
        -e MYSQL_USER="$MYSQL_USER" \
        -e MYSQL_PASSWORD="$MYSQL_PASSWORD" \
        -e MYSQL_DATABASE="$MYSQL_DATABASE" \
        -e REDIS_HOST=paas-redis \
        -e REDIS_PORT="${REDIS_PORT:-6379}" \
        -e REDIS_PASSWORD="$REDIS_PASSWORD" \
        -e JWT_SECRET="$JWT_SECRET" \
        -e BASE_DOMAIN="$BASE_DOMAIN" \
        -e PROJECT_DOMAIN="${PROJECT_DOMAIN:-$BASE_DOMAIN}" \
        -e DOCKER_NETWORK=paas-network \
        --label "traefik.enable=true" \
        --label "traefik.http.routers.backend.rule=Host(\`$BASE_DOMAIN\`) && PathPrefix(\`/api\`)" \
        --label "traefik.http.services.backend.loadbalancer.server.port=8080"
}

deploy_frontend() {
    echo -e "${YELLOW}Deploying frontend module...${NC}"
    FRONTEND_TAG=$(get_next_service_tag "frontend")
    
    deploy_with_anti_downtime "frontend" "${PROJECT_ROOT}/frontend" "$FRONTEND_TAG" \
        --network paas-network \
        --restart unless-stopped \
        --label "traefik.enable=true" \
        --label "traefik.http.routers.frontend.rule=Host(\`$BASE_DOMAIN\`)" \
        --label "traefik.http.services.frontend.loadbalancer.server.port=80"
}

if [[ "$TARGET" == "backend" || "$TARGET" == "all" ]]; then
    deploy_backend
fi

if [[ "$TARGET" == "frontend" || "$TARGET" == "all" ]]; then
    deploy_frontend
fi

echo -e "${GREEN}[SUCCESS] Deployment target '${TARGET}' finished!${NC}"
