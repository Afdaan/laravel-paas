#!/bin/bash
set -e

# ==============================================================================
# Runara CI/CD Deployment Script
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
USER_PG_PASSWORD=${USER_PG_PASSWORD:-"user-pg-rootpassword"}
USER_PG_PORT=${USER_PG_PORT:-5433}
MYSQL_CONTAINER_NAME=${MYSQL_CONTAINER_NAME:-"paas-mysql"}
POSTGRES_CONTAINER_NAME=${POSTGRES_CONTAINER_NAME:-"paas-user-postgres"}
PG_CONTAINER_NAME=${PG_CONTAINER_NAME:-"paas-postgres"}
USER_PG_HOST=${USER_PG_HOST:-"$POSTGRES_CONTAINER_NAME"}

# Deployment Mode
APP_MODE=${APP_MODE:-"docker"}
HOST_ROOT_PATH=${HOST_ROOT_PATH:-"$PROJECT_ROOT"}

# Path initialization for host-side volume mounting
PROJECTS_PATH="${PROJECTS_PATH:-${PROJECT_ROOT}/storage/projects}"
DATA_PATH="${DATA_PATH:-${PROJECT_ROOT}/storage/data}"
TRAEFIK_DYNAMIC_DIR="${TRAEFIK_DYNAMIC_DIR:-${PROJECT_ROOT}/docker/traefik/dynamic}"

# Deployment user identity for deterministic ownership
APP_UID="${APP_UID:-$(id -u)}"
APP_GID="${APP_GID:-$(id -g)}"

# Ensure directories exist
sudo mkdir -p "$PROJECTS_PATH" "$DATA_PATH" "$TRAEFIK_DYNAMIC_DIR"
sudo mkdir -p /nix /var/cache/railpacks
sudo chmod 777 /nix /var/cache/railpacks

# Deterministic storage ownership and permission repair.
# Ensures no root-owned nested dirs block container runtime writes.
repair_storage_permissions() {
    local target="$1"
    if [ ! -d "$target" ]; then
        return
    fi
    sudo chown -R "${APP_UID}:${APP_GID}" "$target"
    find "$target" -type d -exec chmod 775 {} + 2>/dev/null || true
    find "$target" -type f -exec chmod 664 {} + 2>/dev/null || true
}

repair_storage_permissions "$PROJECTS_PATH"
repair_storage_permissions "$DATA_PATH"
sudo chown "${APP_UID}:${APP_GID}" "$TRAEFIK_DYNAMIC_DIR"
chmod 775 "$TRAEFIK_DYNAMIC_DIR"

# Repair existing per-project SQLite dirs/files to safe permissions
if [ -d "$DATA_PATH" ]; then
    find "$DATA_PATH" -mindepth 3 -maxdepth 3 -type d -name "storage" 2>/dev/null | while read -r storage_dir; do
        if [ -d "$storage_dir/sqlite" ]; then
            chmod 775 "$storage_dir/sqlite" 2>/dev/null || true
            if [ -f "$storage_dir/sqlite/database.sqlite" ]; then
                chmod 664 "$storage_dir/sqlite/database.sqlite" 2>/dev/null || true
            fi
        fi
    done
fi

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
    if [ "$service_name" = "worker" ]; then
        container_name="paas-worker-manager"
    fi
    local temp_container_name="${container_name}-new"
    local old_container_name="${container_name}-old"

    echo -e "${YELLOW}[DEPLOY] Working on $service_name (Tag: $image_tag)...${NC}"

    local success=false
    echo -e "${YELLOW}[BUILD] Running docker build with BuildKit... (Retry enabled: 3 attempts)${NC}"

    for attempt in {1..3}; do
        if [ "$service_name" = "backend" ]; then
            if DOCKER_BUILDKIT=1 docker build -t "$image_name" -f "${PROJECT_ROOT}/backend/Dockerfile" "${PROJECT_ROOT}"; then
                success=true
                break
            fi
        elif [ "$service_name" = "worker" ]; then
            if DOCKER_BUILDKIT=1 docker build -t "$image_name" -f "${PROJECT_ROOT}/worker/Dockerfile" "${PROJECT_ROOT}"; then
                success=true
                break
            fi
        else
            if [ "$service_name" = "frontend" ]; then
                # Capture and disable execution tracing to safeguard build arguments from leaking in public CI/CD logs.
                [[ $- == *x* ]] && was_tracing=true || was_tracing=false
                { set +x; } 2>/dev/null

                if DOCKER_BUILDKIT=1 docker build \
                    --build-arg VITE_GITHUB_APP_URL="$VITE_GITHUB_APP_URL" \
                    -t "$image_name" "$context_dir"; then
                    success=true
                fi

                # Restore execution tracing if it was active
                if [ "$was_tracing" = true ]; then set -x; fi

                if [ "$success" = true ]; then
                    break
                fi
            else
                if DOCKER_BUILDKIT=1 docker build -t "$image_name" "$context_dir"; then
                    success=true
                    break
                fi
            fi
        fi
        echo -e "${YELLOW}[WARN] Build attempt $attempt failed. Retrying in 5s...${NC}"
        sleep 5
    done

    if [ "$success" = false ]; then
        echo -e "${RED}[ERROR] Build failed for $service_name after 3 attempts. Keeping current version running.${NC}"
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
        docker tag "$image_name" "paas-$service_name:latest" 2>/dev/null || true

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
        echo -e "${RED}[ERROR] $service_name validation failed. Inspecting container logs before rollback:${NC}"
        docker logs "$temp_container_name" --tail 50 || true
        echo -e "${RED}[ERROR] Rolling back $temp_container_name...${NC}"
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
        -v "${PROJECT_ROOT}/railpacks:/app/railpacks:ro" \
        -v "$TRAEFIK_DYNAMIC_DIR:/etc/traefik/dynamic:rw" \
        -e TRAEFIK_DYNAMIC_DIR=/etc/traefik/dynamic \
        -e APP_MODE="$APP_MODE" \
        -e INTERNAL_API_TOKEN="$INTERNAL_API_TOKEN" \
        -e HOST_ROOT_PATH="$HOST_ROOT_PATH" \
        -e HOST_PROJECTS_PATH="$PROJECTS_PATH" \
        -e HOST_DATA_PATH="$DATA_PATH" \
        -e HOST_TEMPLATES_PATH="${PROJECT_ROOT}/docker/templates" \
        -e HOST_RAILPACKS_PATH="${PROJECT_ROOT}/railpacks" \
        -e PG_HOST="${PG_HOST:-$PG_CONTAINER_NAME}" \
        -e PG_USER="$PG_USER" \
        -e PG_PASSWORD="$PG_PASSWORD" \
        -e PG_DATABASE="$PG_DATABASE" \
        -e MYSQL_HOST="${MYSQL_HOST:-$MYSQL_CONTAINER_NAME}" \
        -e MYSQL_ROOT_PASSWORD="$MYSQL_ROOT_PASSWORD" \
        -e MYSQL_USER="$MYSQL_USER" \
        -e MYSQL_PASSWORD="$MYSQL_PASSWORD" \
        -e MYSQL_DATABASE="$MYSQL_DATABASE" \
        -e REDIS_HOST=paas-redis \
        -e REDIS_PORT="${REDIS_PORT:-6379}" \
        -e REDIS_PASSWORD="$REDIS_PASSWORD" \
        -e JWT_SECRET="$JWT_SECRET" \
        -e UID_SALT="$UID_SALT" \
        -e CREDENTIAL_ENCRYPTION_KEY="$CREDENTIAL_ENCRYPTION_KEY" \
        -e CREDENTIAL_ENCRYPTION_KEY_PREVIOUS="${CREDENTIAL_ENCRYPTION_KEY_PREVIOUS:-}" \
        -e CREDENTIAL_ENCRYPTION_ALLOW_INSECURE_PREVIOUS="${CREDENTIAL_ENCRYPTION_ALLOW_INSECURE_PREVIOUS:-false}" \
        -e BASE_DOMAIN="$BASE_DOMAIN" \
        -e PROJECT_DOMAIN="${PROJECT_DOMAIN:-$BASE_DOMAIN}" \
        -e USER_PG_PASSWORD="$USER_PG_PASSWORD" \
        -e USER_PG_HOST="${USER_PG_HOST:-$POSTGRES_CONTAINER_NAME}" \
        -e USER_PG_PORT="${USER_PG_PORT:-5432}" \
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

deploy_worker() {
    echo -e "${YELLOW}Deploying standalone worker module with zero-downtime...${NC}"
    WORKER_TAG=$(get_next_service_tag "worker")

    echo -e "${YELLOW}Setting target worker version in Redis: $WORKER_TAG...${NC}"
    REDIS_AUTH_PARAM=""
    [ ! -z "$REDIS_PASSWORD" ] && REDIS_AUTH_PARAM="-a $REDIS_PASSWORD"
    docker exec paas-redis redis-cli $REDIS_AUTH_PARAM set "worker:target_version" "$WORKER_TAG" 2>/dev/null || true

    deploy_with_anti_downtime "worker" "${PROJECT_ROOT}" "$WORKER_TAG" \
        --network paas-network \
        --restart unless-stopped \
        -v /var/run/docker.sock:/var/run/docker.sock \
        -v "${PROJECTS_PATH}:/app/storage/projects" \
        -v "${DATA_PATH}:/app/storage/data" \
        -v "${PROJECT_ROOT}/docker/templates:/app/docker/templates:ro" \
        -v "${PROJECT_ROOT}/railpacks:/app/railpacks:ro" \
        -v "$TRAEFIK_DYNAMIC_DIR:/etc/traefik/dynamic:rw" \
        -e TRAEFIK_DYNAMIC_DIR=/etc/traefik/dynamic \
        -e APP_MODE=docker \
        -e HOST_ROOT_PATH="$HOST_ROOT_PATH" \
        -e HOST_PROJECTS_PATH="$PROJECTS_PATH" \
        -e HOST_DATA_PATH="$DATA_PATH" \
        -e HOST_TEMPLATES_PATH="${PROJECT_ROOT}/docker/templates" \
        -e HOST_RAILPACKS_PATH="${PROJECT_ROOT}/railpacks" \
        -e DOCKER_SOCKET=/var/run/docker.sock \
        -e PG_HOST="${PG_HOST:-$PG_CONTAINER_NAME}" \
        -e PG_PORT="${PG_PORT:-5432}" \
        -e PG_USER="$PG_USER" \
        -e PG_PASSWORD="$PG_PASSWORD" \
        -e PG_DATABASE="$PG_DATABASE" \
        -e MYSQL_HOST="${MYSQL_HOST:-$MYSQL_CONTAINER_NAME}" \
        -e REDIS_HOST=paas-redis \
        -e REDIS_PORT="${REDIS_PORT:-6379}" \
        -e REDIS_PASSWORD="$REDIS_PASSWORD" \
        -e UID_SALT="$UID_SALT" \
        -e CREDENTIAL_ENCRYPTION_KEY="$CREDENTIAL_ENCRYPTION_KEY" \
        -e CREDENTIAL_ENCRYPTION_KEY_PREVIOUS="${CREDENTIAL_ENCRYPTION_KEY_PREVIOUS:-}" \
        -e CREDENTIAL_ENCRYPTION_ALLOW_INSECURE_PREVIOUS="${CREDENTIAL_ENCRYPTION_ALLOW_INSECURE_PREVIOUS:-false}" \
        -e BASE_DOMAIN="$BASE_DOMAIN" \
        -e PROJECT_DOMAIN="${PROJECT_DOMAIN:-$BASE_DOMAIN}" \
        -e USER_PG_PASSWORD="$USER_PG_PASSWORD" \
        -e USER_PG_HOST="${USER_PG_HOST:-$POSTGRES_CONTAINER_NAME}" \
        -e USER_PG_PORT="${USER_PG_PORT:-5432}" \
        -e APP_ENV="${APP_ENV:-production}" \
        -e TRUSTED_PROXY_CIDRS="${TRUSTED_PROXY_CIDRS:-}" \
        -e DOCKER_NETWORK=paas-network \
        -e NGINX_WEBHOOK_ENABLED="${NGINX_WEBHOOK_ENABLED:-false}" \
        -e NGINX_WEBHOOK_URL="$NGINX_WEBHOOK_URL" \
        -e NGINX_WEBHOOK_KEY="$NGINX_WEBHOOK_KEY" \
        -e INTERNAL_IP="${INTERNAL_IP:-127.0.0.1}"
}

if [[ "$TARGET" == "backend" || "$TARGET" == "all" ]]; then
    deploy_backend
fi

if [[ "$TARGET" == "worker" || "$TARGET" == "all" ]]; then
    deploy_worker
fi

if [[ "$TARGET" == "frontend" || "$TARGET" == "all" ]]; then
    deploy_frontend
fi

echo -e "${GREEN}[SUCCESS] Deployment target '${TARGET}' finished!${NC}"
