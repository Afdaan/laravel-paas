#!/bin/bash
# ===========================================
# Laravel PaaS Start Script
# ===========================================
# Starts all infrastructure containers
# ===========================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}🚀 Starting Laravel PaaS...${NC}"

# Load environment
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

# Default values
MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PASSWORD:-"rootpassword"}
MYSQL_DATABASE=${MYSQL_DATABASE:-"paas"}
BASE_DOMAIN=${BASE_DOMAIN:-"localhost"}
ACME_EMAIL=${ACME_EMAIL:-"admin@localhost"}

# Create Docker network if not exists
echo -e "${YELLOW}Creating Docker network...${NC}"
docker network create paas-network 2>/dev/null || true

# Start MySQL
echo -e "${YELLOW}Starting MySQL...${NC}"
docker run -d \
    --name paas-mysql \
    --network paas-network \
    --restart unless-stopped \
    -e MYSQL_ROOT_PASSWORD=$MYSQL_ROOT_PASSWORD \
    -e MYSQL_DATABASE=$MYSQL_DATABASE \
    -v paas-mysql-data:/var/lib/mysql \
    mysql:8 \
    2>/dev/null || docker start paas-mysql

# Start Redis
echo -e "${YELLOW}Starting Redis...${NC}"
docker run -d \
    --name paas-redis \
    --network paas-network \
    --restart unless-stopped \
    -v paas-redis-data:/data \
    redis:alpine \
    2>/dev/null || docker start paas-redis

# Start Traefik
echo -e "${YELLOW}Starting Traefik...${NC}"
docker run -d \
    --name paas-traefik \
    --network paas-network \
    --restart unless-stopped \
    -p 80:80 \
    -p 443:443 \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    -v $(pwd)/docker/traefik/traefik.yml:/traefik.yml:ro \
    -v $(pwd)/docker/traefik/dynamic.yml:/etc/traefik/dynamic/dynamic.yml:ro \
    -v paas-letsencrypt:/letsencrypt \
    -e BASE_DOMAIN=$BASE_DOMAIN \
    -e ACME_EMAIL=$ACME_EMAIL \
    traefik:v3.0 \
    2>/dev/null || docker start paas-traefik

# Build and start backend
echo -e "${YELLOW}Building and starting backend...${NC}"
docker build -t paas-backend ./backend
docker run -d \
    --name paas-backend \
    --network paas-network \
    --restart unless-stopped \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v $(pwd)/storage/projects:/app/storage/projects \
    -v $(pwd)/docker/templates:/app/docker/templates:ro \
    -e MYSQL_HOST=paas-mysql \
    -e MYSQL_PASSWORD=$MYSQL_ROOT_PASSWORD \
    -e MYSQL_DATABASE=$MYSQL_DATABASE \
    -e JWT_SECRET=${JWT_SECRET:-$(openssl rand -hex 32)} \
    -e BASE_DOMAIN=$BASE_DOMAIN \
    -e DOCKER_NETWORK=paas-network \
    --label "traefik.enable=true" \
    --label "traefik.http.routers.backend.rule=Host(\`$BASE_DOMAIN\`) && PathPrefix(\`/api\`)" \
    --label "traefik.http.services.backend.loadbalancer.server.port=8080" \
    paas-backend \
    2>/dev/null || docker start paas-backend

# Build and start frontend
echo -e "${YELLOW}Building and starting frontend...${NC}"
docker build -t paas-frontend ./frontend
docker run -d \
    --name paas-frontend \
    --network paas-network \
    --restart unless-stopped \
    --label "traefik.enable=true" \
    --label "traefik.http.routers.frontend.rule=Host(\`$BASE_DOMAIN\`)" \
    --label "traefik.http.services.frontend.loadbalancer.server.port=80" \
    paas-frontend \
    2>/dev/null || docker start paas-frontend

echo -e "${GREEN}✅ Laravel PaaS is running!${NC}"
echo -e "${GREEN}Dashboard: http://$BASE_DOMAIN${NC}"
echo -e "${GREEN}Default login: admin@localhost / admin123${NC}"
