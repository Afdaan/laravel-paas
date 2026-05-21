#!/bin/bash
set -e

# ==============================================================================
# Laravel PaaS Start Script
# Starts all infrastructure containers (MariaDB, Redis, Traefik, Backend, Frontend)
# ==============================================================================

# 1. Environment & Paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
DB_DATA_DIR="${PROJECT_ROOT}/storage/mysql"
PG_DATA_DIR="${PROJECT_ROOT}/storage/postgres"
REDIS_DATA_DIR="${PROJECT_ROOT}/storage/redis"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}[INFO] Starting Laravel PaaS...${NC}"
echo -e "Project root: ${PROJECT_ROOT}"

cd "$PROJECT_ROOT"

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
    # The remaining arguments are docker run flags
    
    local image_name="paas-$service_name:$image_tag"
    local container_name="paas-$service_name"
    local temp_container_name="${container_name}-new"
    local old_container_name="${container_name}-old"

    echo -e "${YELLOW}[DEPLOY] Working on $service_name (Tag: $image_tag)...${NC}"

    # 1. Build new image
    echo -e "${YELLOW}[BUILD] Building $image_name...${NC}"
    if [ "$service_name" = "backend" ]; then
        if ! DOCKER_BUILDKIT=1 docker build -t "$image_name" -f "${PROJECT_ROOT}/backend/Dockerfile" "${PROJECT_ROOT}"; then
            echo -e "${RED}[ERROR] Build failed for $service_name. Keeping current version running.${NC}"
            return 1
        fi
    else
        if ! docker build -t "$image_name" "$context_dir"; then
            echo -e "${RED}[ERROR] Build failed for $service_name. Keeping current version running.${NC}"
            return 1
        fi
    fi
    echo -e "${GREEN}[SUCCESS] Build complete: $image_name${NC}"

    # 2. Start new container as 'new'
    docker rm -f "$temp_container_name" 2>/dev/null || true
    echo -e "${YELLOW}[RUN] Starting new container $temp_container_name...${NC}"
    
    if ! docker run -d --name "$temp_container_name" "$@" "$image_name"; then
        echo -e "${RED}[ERROR] Failed to start new container for $service_name. Keeping current version.${NC}"
        return 1
    fi

    # 3. Wait for health check (max 60s)
    echo -e "${YELLOW}[HEALTH] Waiting for $service_name to be healthy...${NC}"
    local healthy=false
    for i in {1..12}; do
        local status=$(docker inspect --format='{{json .State.Health.Status}}' "$temp_container_name" 2>/dev/null | tr -d '"')
        if [ "$status" == "healthy" ]; then
            healthy=true
            break
        elif [ "$status" == "unhealthy" ]; then
            break
        fi
        # Fallback for containers without healthcheck or still starting
        if [ "$status" == "" ] || [ "$status" == "null" ]; then
             if [ "$(docker inspect -f '{{.State.Running}}' "$temp_container_name" 2>/dev/null)" == "true" ]; then
                # If no health check defined, just wait a bit and assume OK if it didn't crash
                if [ $i -ge 4 ]; then healthy=true; break; fi
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
        
        # 3. NOW stop the old version (it's no longer the primary identity)
        if docker ps -a --format '{{.Names}}' | grep -q "^${old_container_name}$"; then
            echo -e "${YELLOW}[STOP] Stopping previous $service_name version...${NC}"
            docker stop "$old_container_name" 2>/dev/null || true
            docker rm -f "$old_container_name" 2>/dev/null || true
        fi
        
        # 4. Cleanup old images
        echo -e "${YELLOW}[CLEANUP] Removing old images for $service_name...${NC}"
        docker images "paas-$service_name" --format "{{.Tag}}" | grep -v "$image_tag" | xargs -I {} docker rmi "paas-$service_name:{}" 2>/dev/null || true
        
        return 0
    else
        echo -e "${RED}[ERROR] $service_name health check failed. Rolling back...${NC}"
        docker stop "$temp_container_name" 2>/dev/null || true
        docker rm -f "$temp_container_name" 2>/dev/null || true
        return 1
    fi
}

# 2. Variable Initialization
# Load .env if not already set (CI/CD friendly)
if [ -z "$MYSQL_ROOT_PASSWORD" ] && [ -f "$PROJECT_ROOT/.env" ]; then
    echo -e "${YELLOW}Loading .env file...${NC}"
    set -a
    source "$PROJECT_ROOT/.env"
    set +a
    echo -e "${GREEN}[SUCCESS] .env file loaded successfully${NC}"
fi

# Set defaults
MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PASSWORD:-"rootpassword"}
MYSQL_USER=${MYSQL_USER:-"root"}
MYSQL_DATABASE=${MYSQL_DATABASE:-"paas"}
BASE_DOMAIN=${BASE_DOMAIN:-"localhost"}
ACME_EMAIL=${ACME_EMAIL:-"admin@localhost"}
JWT_SECRET=${JWT_SECRET:-"change-me-please-12345"}
MYSQL_PASSWORD=${MYSQL_PASSWORD:-"$MYSQL_ROOT_PASSWORD"}
PG_PASSWORD=${PG_PASSWORD:-"pgrootpassword"}
PG_USER=${PG_USER:-"postgres"}
PG_DATABASE=${PG_DATABASE:-"paas"}
HTTP_PORT=${HTTP_PORT:-80}
HTTPS_PORT=${HTTPS_PORT:-443}
# Deployment Mode
APP_MODE=${APP_MODE:-"docker"}
HOST_ROOT_PATH=${HOST_ROOT_PATH:-"$PROJECT_ROOT"}

# Path initialization for host-side volume mounting
PROJECTS_PATH="${PROJECTS_PATH:-${PROJECT_ROOT}/storage/projects}"
DATA_PATH="${DATA_PATH:-${PROJECT_ROOT}/storage/data}"
TRAEFIK_DYNAMIC_DIR="${TRAEFIK_DYNAMIC_DIR:-${PROJECT_ROOT}/docker/traefik/dynamic}"


# 3. Preparation
echo -e "${YELLOW}Preparing environment...${NC}"
docker network create paas-network 2>/dev/null || true
sudo mkdir -p "$DB_DATA_DIR" "$PG_DATA_DIR" "$REDIS_DATA_DIR" "$PROJECTS_PATH" "$DATA_PATH" "$TRAEFIK_DYNAMIC_DIR"
sudo chown -R $(id -u):$(id -g) "$REDIS_DATA_DIR" "$PROJECTS_PATH" "$DATA_PATH" "$TRAEFIK_DYNAMIC_DIR"
chmod 777 "$DATA_PATH" "$TRAEFIK_DYNAMIC_DIR"


# 4. Smart Backup Logic (Logical or Physical)
LAST_BACKUP_TS="${PROJECT_ROOT}/storage/.last_backup_ts"
CURRENT_TS=$(date +%s)
LAST_TS=0
[ -f "$LAST_BACKUP_TS" ] && LAST_TS=$(cat "$LAST_BACKUP_TS" 2>/dev/null)
SECONDS_SINCE=$((CURRENT_TS - LAST_TS))

# Only backup if FORCE_BACKUP=true OR if last backup is older than 24 hours (86400s)
if [ "$SKIP_BACKUP" != "true" ] && ([ "$FORCE_BACKUP" = "true" ] || [ $SECONDS_SINCE -ge 86400 ]); then
    if docker ps --format '{{.Names}}' | grep -q "^paas-mysql$"; then
        BACKUP_FILE="${PROJECT_ROOT}/storage/mysql-dump-$(date +%Y%m%d-%H%M%S).sql"
        echo -e "${YELLOW}[BACKUP] Running logical backup (mysqldump) for $MYSQL_DATABASE...${NC}"
        docker exec paas-mysql mysqldump -u root -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE" > "$BACKUP_FILE" 2>/dev/null
        if [ $? -eq 0 ]; then
            echo -e "${GREEN}[SUCCESS] Backup complete: $(basename "$BACKUP_FILE")${NC}"
            echo $CURRENT_TS > "$LAST_BACKUP_TS"
        else
            echo -e "${RED}Warning: mysqldump failed, skipping...${NC}"
        fi
    elif [ -d "$DB_DATA_DIR" ] && [ "$(ls -A "$DB_DATA_DIR")" ]; then
        BACKUP_FILE="${PROJECT_ROOT}/storage/mysql-static-$(date +%Y%m%d-%H%M%S).tar"
        echo -e "${YELLOW}[BACKUP] Container offline. Using targeted folder backup...${NC}"
        sudo tar cf "$BACKUP_FILE" -C "$DB_DATA_DIR" "./$MYSQL_DATABASE" 2>/dev/null || true
        sudo chown $(id -u):$(id -g) "$BACKUP_FILE"
        echo $CURRENT_TS > "$LAST_BACKUP_TS"
    fi
else
    echo -e "${GREEN}[SKIP] Skipping redundant backup (Last run: $(((CURRENT_TS-LAST_TS)/60)) mins ago)${NC}"
fi

# 5. Infrastructure: MariaDB
echo -e "${YELLOW}Starting MariaDB...${NC}"
docker rm -f paas-mysql 2>/dev/null || true

# Fix ownership for MariaDB (mapped to UID 999 inside container)
if [ "$(stat -c '%u' "$DB_DATA_DIR")" != "999" ]; then
    echo -e "${YELLOW}Fixing storage/mysql owner to UID 999 (mysql)...${NC}"
    sudo chown -R 999:999 "$DB_DATA_DIR"
fi

docker run -d \
    --name paas-mysql \
    --network paas-network \
    --restart unless-stopped \
    -e MYSQL_ROOT_PASSWORD="$MYSQL_ROOT_PASSWORD" \
    -e MYSQL_DATABASE="$MYSQL_DATABASE" \
    -e MYSQL_USER="$MYSQL_USER" \
    -e MYSQL_PASSWORD="$MYSQL_PASSWORD" \
    -v "${DB_DATA_DIR}:/var/lib/mysql" \
    mariadb:10.11

# 5.5. Infrastructure: PostgreSQL
echo -e "${YELLOW}Starting PostgreSQL...${NC}"
docker rm -f paas-postgres 2>/dev/null || true

docker run -d \
    --name paas-postgres \
    --network paas-network \
    --restart unless-stopped \
    -e POSTGRES_PASSWORD="$PG_PASSWORD" \
    -e POSTGRES_DB="$PG_DATABASE" \
    -e POSTGRES_USER="$PG_USER" \
    -v "${PG_DATA_DIR}:/var/lib/postgresql/data" \
    postgres:15-alpine

# 6. Infrastructure: Redis
echo -e "${YELLOW}Starting Redis with persistence...${NC}"
docker rm -f paas-redis 2>/dev/null || true
REDIS_CMD="redis-server --appendonly yes"
[ ! -z "$REDIS_PASSWORD" ] && REDIS_CMD="$REDIS_CMD --requirepass $REDIS_PASSWORD"
 
 docker run -d \
     --name paas-redis \
     --network paas-network \
     --restart unless-stopped \
     -v "${REDIS_DATA_DIR}:/data" \
     redis:alpine sh -c "$REDIS_CMD"

# 7. Infrastructure: Traefik
echo -e "${YELLOW}Starting Traefik...${NC}"
docker rm -f paas-traefik 2>/dev/null || true

TRAEFIK_CONF="${PROJECT_ROOT}/docker/traefik/traefik.yml"
DYNAMIC_TEMPLATE="${PROJECT_ROOT}/docker/traefik/dynamic.yml.template"
DYNAMIC_CONF="${TRAEFIK_DYNAMIC_DIR}/dynamic.yml"

if [ ! -f "$TRAEFIK_CONF" ]; then
    echo -e "${RED}Error: traefik.yml not found${NC}"
    exit 1
fi

# Generate dynamic config from template
if [ -f "$DYNAMIC_TEMPLATE" ]; then
    sed "s/{{BASE_DOMAIN}}/$BASE_DOMAIN/g" "$DYNAMIC_TEMPLATE" > "$DYNAMIC_CONF"
else
    echo -e "${RED}Error: dynamic.yml.template not found${NC}"
    exit 1
fi

docker run -d \
    --name paas-traefik \
    --network paas-network \
    --restart unless-stopped \
    -p ${HTTP_PORT}:80 \
    -p ${HTTPS_PORT}:443 \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    -v "${TRAEFIK_CONF}:/traefik.yml:ro" \
    -v "${TRAEFIK_DYNAMIC_DIR}:/etc/traefik/dynamic:rw" \
    -v paas-letsencrypt:/letsencrypt \
    traefik:v3.6

# 8. Platform: Backend
echo -e "${YELLOW}Deploying backend with auto-increment tag...${NC}"
if [ ! -d "${PROJECT_ROOT}/backend" ]; then
    echo -e "${RED}Error: backend directory not found${NC}"
    exit 1
fi
BACKEND_TAG=$(get_next_service_tag "backend")

deploy_with_anti_downtime "backend" "${PROJECT_ROOT}/backend" "$BACKEND_TAG" \
    --network paas-network \
    --restart unless-stopped \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "${PROJECT_ROOT}/.env:/app/.env:ro" \
    -v "${PROJECTS_PATH}:/app/storage/projects" \
    -v "${DATA_PATH}:/app/storage/data" \
    -v "${PROJECT_ROOT}/docker/templates:/app/docker/templates:ro" \
    -v "${TRAEFIK_DYNAMIC_DIR}:/etc/traefik/dynamic:rw" \
    -e TRAEFIK_DYNAMIC_DIR=/etc/traefik/dynamic \
    -e APP_MODE="$APP_MODE" \
    -e HOST_ROOT_PATH="$HOST_ROOT_PATH" \
    -e HOST_PROJECTS_PATH="$PROJECTS_PATH" \
    -e HOST_DATA_PATH="$DATA_PATH" \
    -e HOST_TEMPLATES_PATH="${PROJECT_ROOT}/docker/templates" \
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
    --label "traefik.http.services.backend.loadbalancer.server.port=8080" || true

# 8.5. Platform: Standalone Worker Cluster Image
echo -e "${YELLOW}Building standalone worker cluster image...${NC}"
WORKER_TAG=$(get_next_service_tag "worker")
DOCKER_BUILDKIT=1 docker build -t "paas-worker:$WORKER_TAG" -t "paas-worker:latest" -f "${PROJECT_ROOT}/worker/Dockerfile" "${PROJECT_ROOT}" || true
REDIS_AUTH_PARAM=""
[ ! -z "$REDIS_PASSWORD" ] && REDIS_AUTH_PARAM="-a $REDIS_PASSWORD"
docker exec paas-redis redis-cli $REDIS_AUTH_PARAM set "worker:target_version" "$WORKER_TAG" 2>/dev/null || true

echo -e "${YELLOW}[CLEANUP] Pruning outdated worker images...${NC}"
docker images "paas-worker" --format "{{.Tag}}" | grep -E '^[0-9]+$' | grep -v "^${WORKER_TAG}$" | xargs -I {} docker rmi "paas-worker:{}" 2>/dev/null || true

# 8.7. Platform: Standalone Worker Manager Container
echo -e "${YELLOW}Deploying standalone worker manager...${NC}"
docker rm -f paas-worker-manager 2>/dev/null || true
docker run -d \
    --name paas-worker-manager \
    --network paas-network \
    --restart unless-stopped \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "${PROJECTS_PATH}:/app/storage/projects" \
    -v "${DATA_PATH}:/app/data" \
    -v "${PROJECT_ROOT}/docker/templates:/app/docker/templates:ro" \
    -v "${PROJECT_ROOT}/.env:/app/.env:ro" \
    -v "${TRAEFIK_DYNAMIC_DIR}:/etc/traefik/dynamic:rw" \
    -e TRAEFIK_DYNAMIC_DIR=/etc/traefik/dynamic \
    -e APP_MODE=docker \
    -e HOST_ROOT_PATH="$HOST_ROOT_PATH" \
    -e HOST_PROJECTS_PATH="$PROJECTS_PATH" \
    -e HOST_DATA_PATH="$DATA_PATH" \
    -e HOST_TEMPLATES_PATH="${PROJECT_ROOT}/docker/templates" \
    -e DOCKER_SOCKET=/var/run/docker.sock \
    -e PG_HOST=paas-postgres \
    -e PG_PORT=5432 \
    -e PG_USER="$PG_USER" \
    -e PG_PASSWORD="$PG_PASSWORD" \
    -e PG_DATABASE="$PG_DATABASE" \
    -e REDIS_HOST=paas-redis \
    -e REDIS_PORT="${REDIS_PORT:-6379}" \
    -e REDIS_PASSWORD="$REDIS_PASSWORD" \
    -e JWT_SECRET="$JWT_SECRET" \
    -e BASE_DOMAIN="$BASE_DOMAIN" \
    -e PROJECT_DOMAIN="${PROJECT_DOMAIN:-$BASE_DOMAIN}" \
    -e DOCKER_NETWORK=paas-network \
    -e NGINX_WEBHOOK_ENABLED="${NGINX_WEBHOOK_ENABLED:-false}" \
    -e NGINX_WEBHOOK_URL="$NGINX_WEBHOOK_URL" \
    -e NGINX_WEBHOOK_KEY="$NGINX_WEBHOOK_KEY" \
    -e INTERNAL_IP="${INTERNAL_IP:-127.0.0.1}" \
    paas-worker:latest || true

# 9. Platform: Frontend
echo -e "${YELLOW}Deploying frontend with auto-increment tag...${NC}"
if [ ! -d "${PROJECT_ROOT}/frontend" ]; then
    echo -e "${RED}Error: frontend directory not found${NC}"
    exit 1
fi
FRONTEND_TAG=$(get_next_service_tag "frontend")

deploy_with_anti_downtime "frontend" "${PROJECT_ROOT}/frontend" "$FRONTEND_TAG" \
    --network paas-network \
    --restart unless-stopped \
    --label "traefik.enable=true" \
    --label "traefik.http.routers.frontend.rule=Host(\`$BASE_DOMAIN\`)" \
    --label "traefik.http.services.frontend.loadbalancer.server.port=80" || true

echo -e "${GREEN}[SUCCESS] Laravel PaaS is running!${NC}"
if [ "$HTTP_PORT" != "80" ]; then
    echo -e "${GREEN}Dashboard: http://$BASE_DOMAIN:$HTTP_PORT${NC}"
else
    echo -e "${GREEN}Dashboard: http://$BASE_DOMAIN${NC}"
fi
