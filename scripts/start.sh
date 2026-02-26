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
ACME_EMAIL=${ACME_EMAIL:-"admin@localhost"}
JWT_SECRET=${JWT_SECRET:-"change-me-please-12345"}

# Ensure MYSQL_PASSWORD has a value; default to root password if none provided
if [ -z "$MYSQL_PASSWORD" ]; then
    MYSQL_PASSWORD="$MYSQL_ROOT_PASSWORD"
fi

# Create Docker network if not exists
echo -e "${YELLOW}Creating Docker network...${NC}"
docker network create paas-network 2>/dev/null || true

# Start MariaDB (more compatible than MySQL 8)
echo -e "${YELLOW}Starting MariaDB...${NC}"
docker rm -f paas-mysql 2>/dev/null || true
DB_DATA_DIR="${PROJECT_ROOT}/storage/mysql"
# Ensure local storage directory for MySQL exists
mkdir -p "$DB_DATA_DIR"

# Backup MySQL data before starting (safety for production)
if [ -d "$DB_DATA_DIR" ] && [ "$(ls -A "$DB_DATA_DIR")" ]; then
    BACKUP_FILE="${PROJECT_ROOT}/storage/mysql-backup-$(date +%Y%m%d-%H%M%S).tar.gz"
    echo -e "${YELLOW}Backing up MySQL data to $BACKUP_FILE ...${NC}"
    tar czf "$BACKUP_FILE" -C "$DB_DATA_DIR" .
    echo -e "${GREEN}✓ Backup complete${NC}"
fi

# Ensure correct ownership for MariaDB (UID 999)
if [ "$(stat -c '%u' "$DB_DATA_DIR")" != "999" ]; then
    echo -e "${YELLOW}Fixing storage/mysql owner to UID 999 (mysql)...${NC}"
    sudo chown -R 999:999 "$DB_DATA_DIR"
    echo -e "${GREEN}✓ storage/mysql owner fixed${NC}"
fi
docker run -d \
    --name paas-mysql \
    --network paas-network \
    --restart unless-stopped \
    -e MYSQL_ROOT_PASSWORD=$MYSQL_ROOT_PASSWORD \
    -e MYSQL_DATABASE=$MYSQL_DATABASE \
    -e MYSQL_USER=$MYSQL_USER \
    -e MYSQL_PASSWORD=$MYSQL_PASSWORD \
    -v "${PROJECT_ROOT}/storage/mysql:/var/lib/mysql" \
    mariadb:10.11

# Start Redis
echo -e "${YELLOW}Starting Redis...${NC}"
docker rm -f paas-redis 2>/dev/null || true

# Check if REDIS_PASSWORD is set
REDIS_CMD=""
if [ ! -z "$REDIS_PASSWORD" ]; then
    REDIS_CMD="redis-server --requirepass $REDIS_PASSWORD"
fi

docker run -d \
    --name paas-redis \
    --network paas-network \
    --restart unless-stopped \
    -v paas-redis-data:/data \
    redis:alpine $REDIS_CMD

# Start Traefik
echo -e "${YELLOW}Starting Traefik...${NC}"
docker rm -f paas-traefik 2>/dev/null || true

# Check if traefik config exists
if [ ! -f "${PROJECT_ROOT}/docker/traefik/traefik.yml" ]; then
    echo -e "${RED}Error: traefik.yml not found at ${PROJECT_ROOT}/docker/traefik/traefik.yml${NC}"
    exit 1
fi

# Generate dynamic.yml from template with BASE_DOMAIN
echo -e "${YELLOW}Generating Traefik dynamic config...${NC}"
if [ -f "${PROJECT_ROOT}/docker/traefik/dynamic.yml.template" ]; then
    sed "s/{{BASE_DOMAIN}}/$BASE_DOMAIN/g" \
        "${PROJECT_ROOT}/docker/traefik/dynamic.yml.template" > \
        "${PROJECT_ROOT}/docker/traefik/dynamic.yml"
    echo -e "${GREEN}✓ Generated dynamic.yml with BASE_DOMAIN=$BASE_DOMAIN${NC}"
else
    echo -e "${RED}Error: dynamic.yml.template not found${NC}"
    exit 1
fi

docker run -d \
    --name paas-traefik \
    --network paas-network \
    --restart unless-stopped \
    -p 80:80 \
    -p 443:443 \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    -v "${PROJECT_ROOT}/docker/traefik/traefik.yml:/traefik.yml:ro" \
    -v "${PROJECT_ROOT}/docker/traefik/dynamic.yml:/etc/traefik/dynamic/dynamic.yml:ro" \
    -v paas-letsencrypt:/letsencrypt \
    traefik:v3.6

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
    -e MYSQL_HOST=paas-mysql \
    -e MYSQL_USER=$MYSQL_USER \
    -e MYSQL_PASSWORD=$MYSQL_PASSWORD \
    -e MYSQL_DATABASE=$MYSQL_DATABASE \
    -e REDIS_HOST=paas-redis \
    -e REDIS_PORT=${REDIS_PORT:-6379} \
    -e REDIS_PASSWORD=$REDIS_PASSWORD \
    -e JWT_SECRET=$JWT_SECRET \
    -e BASE_DOMAIN=$BASE_DOMAIN \
    -e PROJECT_DOMAIN=${PROJECT_DOMAIN:-$BASE_DOMAIN} \
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
