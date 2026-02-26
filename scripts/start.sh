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

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}🚀 Starting Laravel PaaS...${NC}"
echo -e "Project root: ${PROJECT_ROOT}"

cd "$PROJECT_ROOT"

# 2. Variable Initialization
# Load .env if not already set (CI/CD friendly)
if [ -z "$MYSQL_ROOT_PASSWORD" ] && [ -f "$PROJECT_ROOT/.env" ]; then
    echo -e "${YELLOW}Loading .env file...${NC}"
    set -a
    source "$PROJECT_ROOT/.env"
    set +a
    echo -e "${GREEN}✓ .env file loaded successfully${NC}"
fi

# Set defaults
MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PASSWORD:-"rootpassword"}
MYSQL_USER=${MYSQL_USER:-"root"}
MYSQL_DATABASE=${MYSQL_DATABASE:-"paas"}
BASE_DOMAIN=${BASE_DOMAIN:-"localhost"}
ACME_EMAIL=${ACME_EMAIL:-"admin@localhost"}
JWT_SECRET=${JWT_SECRET:-"change-me-please-12345"}
MYSQL_PASSWORD=${MYSQL_PASSWORD:-"$MYSQL_ROOT_PASSWORD"}

# 3. Preparation
echo -e "${YELLOW}Preparing environment...${NC}"
docker network create paas-network 2>/dev/null || true
mkdir -p "$DB_DATA_DIR"
mkdir -p "${PROJECT_ROOT}/storage/projects"

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
        echo -e "${YELLOW}💾 Running logical backup (mysqldump) for $MYSQL_DATABASE...${NC}"
        docker exec paas-mysql mysqldump -u root -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE" > "$BACKUP_FILE" 2>/dev/null
        if [ $? -eq 0 ]; then
            echo -e "${GREEN}✓ Backup complete: $(basename "$BACKUP_FILE")${NC}"
            echo $CURRENT_TS > "$LAST_BACKUP_TS"
        else
            echo -e "${RED}Warning: mysqldump failed, skipping...${NC}"
        fi
    elif [ -d "$DB_DATA_DIR" ] && [ "$(ls -A "$DB_DATA_DIR")" ]; then
        BACKUP_FILE="${PROJECT_ROOT}/storage/mysql-static-$(date +%Y%m%d-%H%M%S).tar"
        echo -e "${YELLOW}📦 Container offline. Using targeted folder backup...${NC}"
        sudo tar cf "$BACKUP_FILE" -C "$DB_DATA_DIR" "./$MYSQL_DATABASE" 2>/dev/null || true
        sudo chown $(id -u):$(id -g) "$BACKUP_FILE"
        echo $CURRENT_TS > "$LAST_BACKUP_TS"
    fi
else
    echo -e "${GREEN}⏩ Skipping redundant backup (Last run: $(((CURRENT_TS-LAST_TS)/60)) mins ago)${NC}"
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

# 6. Infrastructure: Redis
echo -e "${YELLOW}Starting Redis...${NC}"
docker rm -f paas-redis 2>/dev/null || true
REDIS_CMD=""
[ ! -z "$REDIS_PASSWORD" ] && REDIS_CMD="redis-server --requirepass $REDIS_PASSWORD"

docker run -d \
    --name paas-redis \
    --network paas-network \
    --restart unless-stopped \
    -v paas-redis-data:/data \
    redis:alpine $REDIS_CMD

# 7. Infrastructure: Traefik
echo -e "${YELLOW}Starting Traefik...${NC}"
docker rm -f paas-traefik 2>/dev/null || true

TRAEFIK_CONF="${PROJECT_ROOT}/docker/traefik/traefik.yml"
DYNAMIC_TEMPLATE="${PROJECT_ROOT}/docker/traefik/dynamic.yml.template"
DYNAMIC_CONF="${PROJECT_ROOT}/docker/traefik/dynamic.yml"

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
    -p 80:80 \
    -p 443:443 \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    -v "${TRAEFIK_CONF}:/traefik.yml:ro" \
    -v "${DYNAMIC_CONF}:/etc/traefik/dynamic/dynamic.yml:ro" \
    -v paas-letsencrypt:/letsencrypt \
    traefik:v3.6

# 8. Platform: Backend
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
    --label "traefik.http.services.backend.loadbalancer.server.port=8080" \
    paas-backend

# 9. Platform: Frontend
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
