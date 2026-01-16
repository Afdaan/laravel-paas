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
    # Read .env file line by line, ignore comments and empty lines
    while IFS= read -r line || [ -n "$line" ]; do
        # Ignore comments and empty lines
        if [[ ! "$line" =~ ^# && -n "$line" ]]; then
            # Export the variable
            export "$line"
        fi
    done < "$PROJECT_ROOT/.env"
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
    -v paas-mysql-data:/var/lib/mysql \
    mariadb:10.11

# Start Redis
echo -e "${YELLOW}Starting Redis...${NC}"
docker rm -f paas-redis 2>/dev/null || true
docker run -d \
    --name paas-redis \
    --network paas-network \
    --restart unless-stopped \
    -v paas-redis-data:/data \
    redis:alpine

# Start Traefik
echo -e "${YELLOW}Starting Traefik...${NC}"
docker rm -f paas-traefik 2>/dev/null || true

# Check if traefik config exists
if [ ! -f "${PROJECT_ROOT}/docker/traefik/traefik.yml" ]; then
    echo -e "${RED}Error: traefik.yml not found at ${PROJECT_ROOT}/docker/traefik/traefik.yml${NC}"
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
    -e MYSQL_HOST=paas-mysql \
    -e MYSQL_USER=${MYSQL_USER:-"root"} \
    -e MYSQL_PASSWORD=${MYSQL_ROOT_PASSWORD:-"rootpassword"} \
    -e MYSQL_DATABASE=$MYSQL_DATABASE \
    -e JWT_SECRET=$JWT_SECRET \
    -e BASE_DOMAIN=$BASE_DOMAIN \
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
echo -e "${GREEN}Default login: admin@localhost / admin123${NC}"
