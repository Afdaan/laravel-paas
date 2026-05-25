#!/bin/bash
# ==============================================================================
# Laravel PaaS Start Script
# Starts infrastructure & platform containers selectively or interactively
# ==============================================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 1. Environment & Paths
init_vars() {
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
    DB_DATA_DIR="${PROJECT_ROOT}/storage/mysql"
    PG_DATA_DIR="${PROJECT_ROOT}/storage/postgres"
    REDIS_DATA_DIR="${PROJECT_ROOT}/storage/redis"
    
    cd "$PROJECT_ROOT"

    # Load env vars
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
    APP_MODE=${APP_MODE:-"docker"}
    HOST_ROOT_PATH=${HOST_ROOT_PATH:-"$PROJECT_ROOT"}
    
    PROJECTS_PATH="${PROJECTS_PATH:-${PROJECT_ROOT}/storage/projects}"
    DATA_PATH="${DATA_PATH:-${PROJECT_ROOT}/storage/data}"
    TRAEFIK_DYNAMIC_DIR="${TRAEFIK_DYNAMIC_DIR:-${PROJECT_ROOT}/docker/traefik/dynamic}"
}

prepare_env() {
    docker network create paas-network 2>/dev/null || true
    sudo mkdir -p "$DB_DATA_DIR" "$PG_DATA_DIR" "$REDIS_DATA_DIR" "$PROJECTS_PATH" "$DATA_PATH" "$TRAEFIK_DYNAMIC_DIR"
    sudo chown -R $(id -u):$(id -g) "$REDIS_DATA_DIR" "$PROJECTS_PATH" "$DATA_PATH" "$TRAEFIK_DYNAMIC_DIR"
    sudo chmod 777 "$DATA_PATH" "$TRAEFIK_DYNAMIC_DIR"
}

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

    # Build new image
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

    # Start new container
    docker rm -f "$temp_container_name" 2>/dev/null || true
    echo -e "${YELLOW}[RUN] Starting new container $temp_container_name...${NC}"
    
    if ! docker run -d --name "$temp_container_name" "$@" "$image_name"; then
        echo -e "${RED}[ERROR] Failed to start new container for $service_name. Keeping current version.${NC}"
        return 1
    fi

    # Wait for health check (max 60s)
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
        if [ "$status" == "" ] || [ "$status" == "null" ]; then
             if [ "$(docker inspect -f '{{.State.Running}}' "$temp_container_name" 2>/dev/null)" == "true" ]; then
                if [ $i -ge 4 ]; then healthy=true; break; fi
             fi
        fi
        sleep 5
    done

    if [ "$healthy" == "true" ]; then
        echo -e "${GREEN}[SUCCESS] $service_name is healthy! Swapping containers...${NC}"
        if docker ps -a --format '{{.Names}}' | grep -q "^${container_name}$"; then
            docker rename "$container_name" "$old_container_name" 2>/dev/null || true
        fi
        docker rename "$temp_container_name" "$container_name"
        if docker ps -a --format '{{.Names}}' | grep -q "^${old_container_name}$"; then
            docker stop "$old_container_name" 2>/dev/null || true
            docker rm -f "$old_container_name" 2>/dev/null || true
        fi
        docker images "paas-$service_name" --format "{{.Tag}}" | grep -v "$image_tag" | xargs -I {} docker rmi "paas-$service_name:{}" 2>/dev/null || true
        return 0
    else
        echo -e "${RED}[ERROR] $service_name health check failed. Rolling back...${NC}"
        docker stop "$temp_container_name" 2>/dev/null || true
        docker rm -f "$temp_container_name" 2>/dev/null || true
        return 1
    fi
}

run_backups() {
    LAST_BACKUP_TS="${PROJECT_ROOT}/storage/.last_backup_ts"
    CURRENT_TS=$(date +%s)
    LAST_TS=0
    [ -f "$LAST_BACKUP_TS" ] && LAST_TS=$(cat "$LAST_BACKUP_TS" 2>/dev/null)
    SECONDS_SINCE=$((CURRENT_TS - LAST_TS))

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
}

# 3. Individual Service Startup Blocks
start_mysql() {
    echo -e "${YELLOW}Starting MariaDB...${NC}"
    docker rm -f paas-mysql 2>/dev/null || true
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
}

start_postgres() {
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
}

start_redis() {
    echo -e "${YELLOW}Starting Redis with persistence...${NC}"
    docker rm -f paas-redis 2>/dev/null || true
    local redis_cmd="redis-server --appendonly yes"
    [ ! -z "$REDIS_PASSWORD" ] && redis_cmd="$redis_cmd --requirepass $REDIS_PASSWORD"
    docker run -d \
        --name paas-redis \
        --network paas-network \
        --restart unless-stopped \
        -v "${REDIS_DATA_DIR}:/data" \
        redis:alpine sh -c "$redis_cmd"
}

start_buildkit() {
    echo -e "${YELLOW}Starting BuildKit (Rootless)...${NC}"
    docker rm -f paas-buildkit 2>/dev/null || true
    docker volume create paas-buildkit-cache 2>/dev/null || true
    local config_path="${PROJECT_ROOT}/docker/templates/buildkitd.toml"
    docker run -d \
        --name paas-buildkit \
        --network paas-network \
        -p 127.0.0.1:1234:1234 \
        --security-opt seccomp=unconfined \
        --security-opt apparmor=unconfined \
        --restart unless-stopped \
        --cpus="2.0" \
        --memory="3g" \
        -v paas-buildkit-cache:/var/lib/buildkit \
        -v "${config_path}:/etc/buildkit/buildkitd.toml:ro" \
        moby/buildkit:rootless --addr tcp://0.0.0.0:1234
}


start_traefik() {
    echo -e "${YELLOW}Starting Traefik...${NC}"
    docker rm -f paas-traefik 2>/dev/null || true
    local traefik_conf="${PROJECT_ROOT}/docker/traefik/traefik.yml"
    local dynamic_template="${PROJECT_ROOT}/docker/traefik/dynamic.yml.template"
    local dynamic_conf="${TRAEFIK_DYNAMIC_DIR}/dynamic.yml"

    if [ ! -f "$traefik_conf" ]; then
        echo -e "${RED}Error: traefik.yml not found${NC}"
        return 1
    fi
    if [ -f "$dynamic_template" ]; then
        sed "s/{{BASE_DOMAIN}}/$BASE_DOMAIN/g" "$dynamic_template" > "$dynamic_conf"
    else
        echo -e "${RED}Error: dynamic.yml.template not found${NC}"
        return 1
    fi

    docker run -d \
        --name paas-traefik \
        --network paas-network \
        --restart unless-stopped \
        -p ${HTTP_PORT}:80 \
        -p ${HTTPS_PORT}:443 \
        -v /var/run/docker.sock:/var/run/docker.sock:ro \
        -v "${traefik_conf}:/traefik.yml:ro" \
        -v "${TRAEFIK_DYNAMIC_DIR}:/etc/traefik/dynamic:rw" \
        -v paas-letsencrypt:/letsencrypt \
        traefik:v3.6
}

start_backend() {
    echo -e "${YELLOW}Deploying backend with auto-increment tag...${NC}"
    if [ ! -d "${PROJECT_ROOT}/backend" ]; then
        echo -e "${RED}Error: backend directory not found${NC}"
        return 1
    fi
    local backend_tag=$(get_next_service_tag "backend")
    deploy_with_anti_downtime "backend" "${PROJECT_ROOT}/backend" "$backend_tag" \
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
        --label "traefik.http.services.backend.loadbalancer.server.port=8080"
}

start_worker() {
    echo -e "${YELLOW}Building standalone worker cluster image...${NC}"
    local worker_tag=$(get_next_service_tag "worker")
    DOCKER_BUILDKIT=1 docker build -t "paas-worker:$worker_tag" -t "paas-worker:latest" -f "${PROJECT_ROOT}/worker/Dockerfile" "${PROJECT_ROOT}" || true
    local redis_auth_param=""
    [ ! -z "$REDIS_PASSWORD" ] && redis_auth_param="-a $REDIS_PASSWORD"
    docker exec paas-redis redis-cli $redis_auth_param set "worker:target_version" "$worker_tag" 2>/dev/null || true

    echo -e "${YELLOW}[CLEANUP] Pruning outdated worker images...${NC}"
    docker images "paas-worker" --format "{{.Tag}}" | grep -E '^[0-9]+$' | grep -v "^${worker_tag}$" | xargs -I {} docker rmi "paas-worker:{}" 2>/dev/null || true

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
        paas-worker:latest
}

start_frontend() {
    echo -e "${YELLOW}Deploying frontend with auto-increment tag...${NC}"
    if [ ! -d "${PROJECT_ROOT}/frontend" ]; then
        echo -e "${RED}Error: frontend directory not found${NC}"
        return 1
    fi
    local frontend_tag=$(get_next_service_tag "frontend")
    deploy_with_anti_downtime "frontend" "${PROJECT_ROOT}/frontend" "$frontend_tag" \
        --network paas-network \
        --restart unless-stopped \
        --label "traefik.enable=true" \
        --label "traefik.http.routers.frontend.rule=Host(\`$BASE_DOMAIN\`)" \
        --label "traefik.http.services.frontend.loadbalancer.server.port=80"
}

start_all() {
    prepare_env
    run_backups
    start_mysql
    start_postgres
    start_redis
    start_traefik
    start_buildkit
    start_backend
    start_worker
    start_frontend
    echo -e "${GREEN}[SUCCESS] Laravel PaaS is running!${NC}"
    if [ "$HTTP_PORT" != "80" ]; then
        echo -e "${GREEN}Dashboard: http://$BASE_DOMAIN:$HTTP_PORT${NC}"
    else
        echo -e "${GREEN}Dashboard: http://$BASE_DOMAIN${NC}"
    fi
}

start_service() {
    prepare_env
    case "$1" in
        mysql) start_mysql ;;
        postgres|psql) start_postgres ;;
        redis) start_redis ;;
        traefik) start_traefik ;;
        buildkit) start_buildkit ;;
        backend) start_backend ;;
        worker) start_worker ;;
        frontend) start_frontend ;;
        *) echo -e "${RED}Unknown service: $1${NC}" ;;
    esac
}

# 4. Status Checker
show_status() {
    echo -e "\n${GREEN}=== Laravel PaaS Container Status ===${NC}"
    echo -e "------------------------------------------------------------"
    printf " %-22s | %-18s | %-15s\n" "Service Name" "Status" "IP Address"
    echo -e "------------------------------------------------------------"
    local services=("paas-mysql" "paas-postgres" "paas-redis" "paas-traefik" "paas-buildkit" "paas-backend" "paas-worker-manager" "paas-frontend")
    for s in "${services[@]}"; do
        local status="not_created"
        local ip="-"
        if docker ps -a --format '{{.Names}}' | grep -q "^${s}$"; then
            status=$(docker inspect -f '{{.State.Status}}' "$s" 2>/dev/null || echo "unknown")
            ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$s" 2>/dev/null || echo "-")
        fi
        [ -z "$ip" ] && ip="-"
        
        local status_color=$RED
        if [ "$status" = "running" ]; then
            status_color=$GREEN
            local health=$(docker inspect -f '{{.State.Health.Status}}' "$s" 2>/dev/null || echo "")
            if [ -n "$health" ] && [ "$health" != "<nil>" ] && [ "$health" != "null" ]; then
                status="running ($health)"
                if [ "$health" != "healthy" ]; then
                    status_color=$YELLOW
                fi
            fi
        fi
        
        printf " %-22s | %b%-18s%b | %-15s\n" "$s" "$status_color" "$status" "$NC" "$ip"
    done
    echo -e "------------------------------------------------------------\n"
}

# 5. UI Menus
service_menu() {
    while true; do
        echo -e "\n${YELLOW}=== Start Individual Service ===${NC}"
        echo "1) MySQL (paas-mysql)"
        echo "2) PostgreSQL (paas-postgres)"
        echo "3) Redis (paas-redis)"
        echo "4) Traefik (paas-traefik)"
        echo "5) BuildKit (paas-buildkit)"
        echo "6) Backend (paas-backend)"
        echo "7) Worker Manager (paas-worker-manager)"
        echo "8) Frontend (paas-frontend)"
        echo "9) Back to Main Menu"
        read -p "Select service [1-9]: " s_opt
        
        case "$s_opt" in
            1) start_mysql ; break ;;
            2) start_postgres ; break ;;
            3) start_redis ; break ;;
            4) start_traefik ; break ;;
            5) start_buildkit ; break ;;
            6) start_backend ; break ;;
            7) start_worker ; break ;;
            8) start_frontend ; break ;;
            9) return 0 ;;
            *) echo -e "${RED}Invalid option!${NC}" ;;
        esac
    done
}

interactive_menu() {
    while true; do
        echo -e "${YELLOW}=== Laravel PaaS Startup Panel ===${NC}"
        echo "1) Start/Restart All Services"
        echo "2) Start/Restart Specific Service"
        echo "3) Show Container Status"
        echo "4) Exit"
        read -p "Select option [1-4]: " main_opt
        
        case "$main_opt" in
            1) start_all ;;
            2) service_menu ;;
            3) show_status ;;
            4|q|Q) echo "Exiting." && exit 0 ;;
            *) echo -e "${RED}Invalid option!${NC}\n" ;;
        esac
    done
}

# 6. Main Flow Execution
init_vars

case "$1" in
    all)
        start_all
        ;;
    mysql|postgres|psql|redis|traefik|backend|worker|frontend)
        start_service "$1"
        ;;
    status)
        show_status
        ;;
    "")
        interactive_menu
        ;;
    *)
        echo "Usage: $0 [all|service_name|status]"
        echo "Services: mysql, postgres/psql, redis, traefik, buildkit, backend, worker, frontend"
        exit 1
        ;;
esac
