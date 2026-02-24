#!/bin/bash
# ===========================================
# Laravel PaaS Start Script
# ===========================================
# Starts all infrastructure containers
# ===========================================

set -e

# Get script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}🚀 Starting Laravel PaaS...${NC}"
echo -e "Project root: ${PROJECT_ROOT}"

# Change to project root
cd "$PROJECT_ROOT"

# Load environment variables from .env file
if [ -f "$PROJECT_ROOT/.env" ]; then
    echo -e "${YELLOW}Loading .env file...${NC}"
    # Use set -a to auto-export all variables, then source the file
    set -a
    source "$PROJECT_ROOT/.env"
    set +a
    echo -e "${GREEN}✓ .env file loaded successfully${NC}"
else
    echo -e "${YELLOW}Warning: .env file not found at $PROJECT_ROOT/.env${NC}"
fi

# Set default values if not provided in .env
MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PASSWORD:-"rootpassword"}
MYSQL_USER=${MYSQL_USER:-"root"}
MYSQL_DATABASE=${MYSQL_DATABASE:-"paas"}
BASE_DOMAIN=${BASE_DOMAIN:-"localhost"}
PROJECT_DOMAIN=${PROJECT_DOMAIN:-$BASE_DOMAIN}
ACME_EMAIL=${ACME_EMAIL:-"admin@localhost"}
JWT_SECRET=${JWT_SECRET:-"change-me-please-12345"}

# Create Docker network if not exists
echo -e "${YELLOW}Creating Docker network...${NC}"
docker network create paas-network 2>/dev/null || true

# Start MariaDB (more compatible than MySQL 8)
echo -e "${YELLOW}Starting MariaDB...${NC}"
docker rm -f paas-mysql 2>/dev/null || true
docker run -d \
    --name paas-mysql \
    --network paas-network \
    --restart unless-stopped \
    -e MYSQL_ROOT_PASSWORD=$MYSQL_ROOT_PASSWORD \
    -e MYSQL_DATABASE=$MYSQL_DATABASE \
    -e MYSQL_USER=$MYSQL_USER \
    -e MYSQL_PASSWORD=$MYSQL_PASSWORD \
    -v paas-mysql-data:/var/lib/mysql \
    mariadb:10.11


# Start Redis
echo -e "${YELLOW}Starting Redis...${NC}"
docker rm -f paas-redis 2>/dev/null || true
REDIS_ARGS=()
if [ ! -z "$REDIS_PASSWORD" ]; then
    REDIS_ARGS=("redis-server" "--requirepass" "$REDIS_PASSWORD")
fi
if [ ${#REDIS_ARGS[@]} -eq 0 ]; then
    docker run -d \
        --name paas-redis \
        --network paas-network \
        --restart unless-stopped \
        -v paas-redis-data:/data \
        redis:alpine
else
    docker run -d \
        --name paas-redis \
        --network paas-network \
        --restart unless-stopped \
        -v paas-redis-data:/data \
        redis:alpine "${REDIS_ARGS[@]}"
fi

# Start Traefik
echo -e "${YELLOW}Starting Traefik...${NC}"
docker rm -f paas-traefik 2>/dev/null || true

# Check if traefik config exists



echo -e "${YELLOW}Preparing Traefik dynamic config directory...${NC}"
TRAEFIK_DYNAMIC_DIR="${PROJECT_ROOT}/docker/traefik/dynamic"
TRAEFIK_DYNAMIC_TEMPLATE="${PROJECT_ROOT}/docker/traefik/dynamic.yml.template"
TRAEFIK_DYNAMIC_FILE="${TRAEFIK_DYNAMIC_DIR}/dynamic.yml"

mkdir -p "${TRAEFIK_DYNAMIC_DIR}"

# Only generate a default dynamic.yml if it doesn't exist.
# Source of truth after first boot is DB (panel settings) and backend will regenerate on startup + on updates.
if [ -f "${TRAEFIK_DYNAMIC_FILE}" ]; then
    echo -e "${GREEN}✓ Keeping existing dynamic.yml (won't overwrite panel/DB settings)${NC}"
else
    echo -e "${YELLOW}Generating initial dynamic.yml from .env defaults...${NC}"
    if [ -f "${TRAEFIK_DYNAMIC_TEMPLATE}" ]; then
        sed \
            -e "s/{{BASE_DOMAIN}}/$BASE_DOMAIN/g" \
            -e "s/{{PROJECT_DOMAIN}}/$PROJECT_DOMAIN/g" \
            "${TRAEFIK_DYNAMIC_TEMPLATE}" > \
            "${TRAEFIK_DYNAMIC_FILE}"
        echo -e "${GREEN}✓ Generated initial dynamic.yml with BASE_DOMAIN=$BASE_DOMAIN PROJECT_DOMAIN=$PROJECT_DOMAIN${NC}"
    else
        echo -e "${RED}Error: dynamic.yml.template not found${NC}"
        exit 1
    fi
fi

docker run -d \
    --name paas-traefik \
    --network paas-network \
    --restart unless-stopped \
    -p 80:80 \
    -v "${PROJECT_ROOT}/docker/traefik/traefik.yml:/traefik.yml:ro" \
    -v "${PROJECT_ROOT}/docker/traefik/dynamic:/etc/traefik/dynamic:ro" \
    -v paas-letsencrypt:/letsencrypt \
    traefik:v3.0

# Build and start backend
echo -e "${YELLOW}Building and starting backend...${NC}"
if [ ! -d "${PROJECT_ROOT}/backend" ]; then
    echo -e "${RED}Error: backend directory not found${NC}"
    exit 1
fi

docker rm -f paas-backend 2>/dev/null || true
docker build -t paas-backend "${PROJECT_ROOT}/backend"
docker run -d \
    --name paas-backend \
    --network paas-network \
    --restart unless-stopped \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "${PROJECT_ROOT}/.env:/app/.env:ro" \
    -v "${PROJECT_ROOT}/storage/projects:/app/storage/projects" \
    -v "${PROJECT_ROOT}/docker/templates:/app/docker/templates:ro" \
    -v "${PROJECT_ROOT}/docker/traefik:/app/docker/traefik" \
    -e MYSQL_HOST=paas-mysql \
    -e MYSQL_USER=${MYSQL_USER:-"root"} \
    -e MYSQL_PASSWORD=${MYSQL_ROOT_PASSWORD:-"rootpassword"} \
    -e MYSQL_DATABASE=$MYSQL_DATABASE \
    -e REDIS_HOST=paas-redis \
    -e REDIS_PORT=${REDIS_PORT:-6379} \
    -e REDIS_PASSWORD=$REDIS_PASSWORD \
    -e JWT_SECRET=$JWT_SECRET \
    -e BASE_DOMAIN=$BASE_DOMAIN \
    -e PROJECT_DOMAIN=$PROJECT_DOMAIN \
    -e TRAEFIK_DYNAMIC_TEMPLATE_PATH=/app/docker/traefik/dynamic.yml.template \
    -e TRAEFIK_DYNAMIC_CONFIG_PATH=/app/docker/traefik/dynamic/dynamic.yml \
    -e DOCKER_NETWORK=paas-network \
    --label "traefik.enable=true" \
    --label "traefik.http.routers.backend.rule=Host(\`$BASE_DOMAIN\`) && PathPrefix(\`/api\`)" \
    --label "traefik.http.services.backend.loadbalancer.server.port=8080" \
    paas-backend

# Build and start frontend
echo -e "${YELLOW}Building and starting frontend...${NC}"
if [ ! -d "${PROJECT_ROOT}/frontend" ]; then
    echo -e "${RED}Error: frontend directory not found${NC}"
    exit 1
fi

docker rm -f paas-frontend 2>/dev/null || true
docker build -t paas-frontend "${PROJECT_ROOT}/frontend"
docker run -d \
    --name paas-frontend \
    --network paas-network \
    --restart unless-stopped \
    --label "traefik.enable=true" \
    --label "traefik.http.routers.frontend.rule=Host(\`$BASE_DOMAIN\`)" \
    --label "traefik.http.services.frontend.loadbalancer.server.port=80" \
    paas-frontend

echo -e "${GREEN}✅ Laravel PaaS is running!${NC}"
echo -e "${GREEN}Dashboard: http://$BASE_DOMAIN${NC}"
