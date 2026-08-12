#!/bin/bash
# ==============================================================================
# Runara Management & Control Panel Script
# Allows selective, interactive, or argument-driven management of all services
# ==============================================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/traefik-config.sh"

# 1. Environment & Paths Initialization
init_vars() {
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
    DB_DATA_DIR="${PROJECT_ROOT}/storage/mysql"
    PG_DATA_DIR="${PROJECT_ROOT}/storage/postgres"
    REDIS_DATA_DIR="${PROJECT_ROOT}/storage/redis"
    USER_PG_DATA_DIR="${PROJECT_ROOT}/storage/user-postgres"

    cd "$PROJECT_ROOT"

    # Load env vars
    if [ -z "$MYSQL_ROOT_PASSWORD" ] && [ -f "$PROJECT_ROOT/.env" ]; then
        set -a
        source "$PROJECT_ROOT/.env"
        set +a
    fi

    # Set default variables (matching start.sh)
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
    USER_PG_PASSWORD=${USER_PG_PASSWORD:-"user-pg-rootpassword"}
    USER_PG_PORT=${USER_PG_PORT:-5433}
    HTTP_PORT=${HTTP_PORT:-80}
    HTTPS_PORT=${HTTPS_PORT:-443}
    APP_MODE=${APP_MODE:-"docker"}
    HOST_ROOT_PATH=${HOST_ROOT_PATH:-"$PROJECT_ROOT"}
    MYSQL_CONTAINER_NAME=${MYSQL_CONTAINER_NAME:-"paas-mysql"}
    POSTGRES_CONTAINER_NAME=${POSTGRES_CONTAINER_NAME:-"paas-user-postgres"}

    PROJECTS_PATH="${PROJECTS_PATH:-${PROJECT_ROOT}/storage/projects}"
    DATA_PATH="${DATA_PATH:-${PROJECT_ROOT}/storage/data}"
    TRAEFIK_DYNAMIC_DIR="${TRAEFIK_DYNAMIC_DIR:-${PROJECT_ROOT}/docker/traefik/dynamic}"
}

prepare_env() {
    docker network create paas-network 2>/dev/null || true
    sudo mkdir -p "$DB_DATA_DIR" "$PG_DATA_DIR" "$USER_PG_DATA_DIR" "$REDIS_DATA_DIR" "$PROJECTS_PATH" "$DATA_PATH" "$TRAEFIK_DYNAMIC_DIR"
    sudo chown -R $(id -u):$(id -g) "$REDIS_DATA_DIR" "$PROJECTS_PATH" "$DATA_PATH" "$TRAEFIK_DYNAMIC_DIR"
    chmod 777 "$DATA_PATH"
    chmod 700 "$TRAEFIK_DYNAMIC_DIR"
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

# Helper to deploy with zero downtime
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
    echo -e "${YELLOW}[HEALTH] Checking health of new container...${NC}"
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

# 2. Individual Service Start/Stop Controllers
start_mysql() {
    echo -e "${YELLOW}Starting MariaDB ($MYSQL_CONTAINER_NAME)...${NC}"
    prepare_env
    docker rm -f "$MYSQL_CONTAINER_NAME" 2>/dev/null || true
    if [ "$(stat -c '%u' "$DB_DATA_DIR")" != "999" ]; then
        sudo chown -R 999:999 "$DB_DATA_DIR"
    fi
    docker run -d \
        --name "$MYSQL_CONTAINER_NAME" \
        --network paas-network \
        --restart unless-stopped \
        -e MYSQL_ROOT_PASSWORD="$MYSQL_ROOT_PASSWORD" \
        -e MYSQL_DATABASE="$MYSQL_DATABASE" \
        -e MYSQL_USER="$MYSQL_USER" \
        -e MYSQL_PASSWORD="$MYSQL_PASSWORD" \
        -v "${DB_DATA_DIR}:/var/lib/mysql" \
        mariadb:10.11
}

stop_mysql() {
    echo -e "${YELLOW}Stopping MariaDB ($MYSQL_CONTAINER_NAME)...${NC}"
    docker stop "$MYSQL_CONTAINER_NAME" 2>/dev/null || true
    docker rm "$MYSQL_CONTAINER_NAME" 2>/dev/null || true
}

start_postgres() {
    echo -e "${YELLOW}Starting PostgreSQL (paas-postgres)...${NC}"
    prepare_env
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

stop_postgres() {
    echo -e "${YELLOW}Stopping PostgreSQL (paas-postgres)...${NC}"
    docker stop paas-postgres 2>/dev/null || true
    docker rm paas-postgres 2>/dev/null || true
}

start_user_postgres() {
    echo -e "${YELLOW}Starting User PostgreSQL ($POSTGRES_CONTAINER_NAME)...${NC}"
    prepare_env
    docker rm -f "$POSTGRES_CONTAINER_NAME" 2>/dev/null || true
    docker run -d \
        --name "$POSTGRES_CONTAINER_NAME" \
        --network paas-network \
        --restart unless-stopped \
        -e POSTGRES_USER="postgres" \
        -e POSTGRES_PASSWORD="$USER_PG_PASSWORD" \
        -e POSTGRES_DB="postgres" \
        -p "$USER_PG_PORT":5432 \
        -v "${USER_PG_DATA_DIR}:/var/lib/postgresql/data" \
        postgres:15-alpine
}

stop_user_postgres() {
    echo -e "${YELLOW}Stopping User PostgreSQL ($POSTGRES_CONTAINER_NAME)...${NC}"
    docker stop "$POSTGRES_CONTAINER_NAME" 2>/dev/null || true
    docker rm "$POSTGRES_CONTAINER_NAME" 2>/dev/null || true
}

start_redis() {
    echo -e "${YELLOW}Starting Redis (paas-redis)...${NC}"
    prepare_env
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

stop_redis() {
    echo -e "${YELLOW}Stopping Redis (paas-redis)...${NC}"
    docker stop paas-redis 2>/dev/null || true
    docker rm paas-redis 2>/dev/null || true
}

start_traefik() {
    echo -e "${YELLOW}Starting Traefik (paas-traefik)...${NC}"
    prepare_env
    docker rm -f paas-traefik 2>/dev/null || true
    local traefik_template="${PROJECT_ROOT}/docker/traefik/traefik.yml.template"
    local traefik_conf
    local dynamic_template="${PROJECT_ROOT}/docker/traefik/dynamic.yml.template"
    local dynamic_conf="${TRAEFIK_DYNAMIC_DIR}/dynamic.yml"

    if ! traefik_conf=$(render_traefik_static_config "$traefik_template"); then
        echo -e "${RED}Error: failed to render Traefik static config.${NC}"
        return 1
    fi
    if [ -f "$dynamic_template" ]; then
        sed -e "s/{{BASE_DOMAIN}}/$BASE_DOMAIN/g" -e "s/{{PROJECT_DOMAIN}}/${PROJECT_DOMAIN:-$BASE_DOMAIN}/g" "$dynamic_template" > "$dynamic_conf"
    else
        echo -e "${RED}Error: dynamic.yml.template not found at $dynamic_template${NC}"
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
        -v "${TRAEFIK_DYNAMIC_DIR}:/etc/traefik/dynamic:ro" \
        -v paas-letsencrypt:/letsencrypt \
        traefik:v3.6
}

stop_traefik() {
    echo -e "${YELLOW}Stopping Traefik (paas-traefik)...${NC}"
    docker stop paas-traefik 2>/dev/null || true
    docker rm paas-traefik 2>/dev/null || true
}

start_backend() {
    echo -e "${YELLOW}Starting Backend (paas-backend)...${NC}"
    prepare_env
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
        -v "${PROJECT_ROOT}/railpacks:/app/railpacks:ro" \
        -v "${TRAEFIK_DYNAMIC_DIR}:/etc/traefik/dynamic:rw" \
        -e TRAEFIK_DYNAMIC_DIR=/etc/traefik/dynamic \
        -e APP_MODE="$APP_MODE" \
        -e INTERNAL_API_TOKEN="$INTERNAL_API_TOKEN" \
        -e HOST_ROOT_PATH="$HOST_ROOT_PATH" \
        -e HOST_PROJECTS_PATH="$PROJECTS_PATH" \
        -e HOST_DATA_PATH="$DATA_PATH" \
        -e HOST_TEMPLATES_PATH="${PROJECT_ROOT}/docker/templates" \
        -e HOST_RAILPACKS_PATH="${PROJECT_ROOT}/railpacks" \
        -e PG_HOST=paas-postgres \
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

stop_backend() {
    echo -e "${YELLOW}Stopping Backend (paas-backend)...${NC}"
    docker stop paas-backend 2>/dev/null || true
    docker rm paas-backend 2>/dev/null || true
}

start_worker() {
    echo -e "${YELLOW}Starting Worker Manager (paas-worker-manager)...${NC}"
    prepare_env
    local worker_tag=$(get_next_service_tag "worker")
    DOCKER_BUILDKIT=1 docker build -t "paas-worker:$worker_tag" -t "paas-worker:latest" -f "${PROJECT_ROOT}/worker/Dockerfile" "${PROJECT_ROOT}"

    local redis_auth_param=""
    [ ! -z "$REDIS_PASSWORD" ] && redis_auth_param="-a $REDIS_PASSWORD"
    docker exec paas-redis redis-cli $redis_auth_param set "worker:target_version" "$worker_tag" 2>/dev/null || true

    docker images "paas-worker" --format "{{.Tag}}" | grep -E '^[0-9]+$' | grep -v "^${worker_tag}$" | xargs -I {} docker rmi "paas-worker:{}" 2>/dev/null || true

    docker rm -f paas-worker-manager 2>/dev/null || true
    docker run -d \
        --name paas-worker-manager \
        --network paas-network \
        --restart unless-stopped \
        -v /var/run/docker.sock:/var/run/docker.sock \
        -v "${PROJECTS_PATH}:/app/storage/projects" \
        -v "${DATA_PATH}:/app/data" \
        -v "${PROJECT_ROOT}/docker/templates:/app/docker/templates:ro" \
        -v "${PROJECT_ROOT}/railpacks:/app/railpacks:ro" \
        -v "${TRAEFIK_DYNAMIC_DIR}:/etc/traefik/dynamic:rw" \
        -e TRAEFIK_DYNAMIC_DIR=/etc/traefik/dynamic \
        -e APP_MODE=docker \
        -e HOST_ROOT_PATH="$HOST_ROOT_PATH" \
        -e HOST_PROJECTS_PATH="$PROJECTS_PATH" \
        -e HOST_DATA_PATH="$DATA_PATH" \
        -e HOST_TEMPLATES_PATH="${PROJECT_ROOT}/docker/templates" \
        -e HOST_RAILPACKS_PATH="${PROJECT_ROOT}/railpacks" \
        -e DOCKER_SOCKET=/var/run/docker.sock \
        -e PG_HOST=paas-postgres \
        -e PG_PORT=5432 \
        -e PG_USER="$PG_USER" \
        -e PG_PASSWORD="$PG_PASSWORD" \
        -e PG_DATABASE="$PG_DATABASE" \
        -e REDIS_HOST=paas-redis \
        -e REDIS_PORT="${REDIS_PORT:-6379}" \
        -e REDIS_PASSWORD="$REDIS_PASSWORD" \
        -e UID_SALT="$UID_SALT" \
        -e CREDENTIAL_ENCRYPTION_KEY="$CREDENTIAL_ENCRYPTION_KEY" \
        -e CREDENTIAL_ENCRYPTION_KEY_PREVIOUS="${CREDENTIAL_ENCRYPTION_KEY_PREVIOUS:-}" \
        -e CREDENTIAL_ENCRYPTION_ALLOW_INSECURE_PREVIOUS="${CREDENTIAL_ENCRYPTION_ALLOW_INSECURE_PREVIOUS:-false}" \
        -e BASE_DOMAIN="$BASE_DOMAIN" \
        -e PROJECT_DOMAIN="${PROJECT_DOMAIN:-$BASE_DOMAIN}" \
        -e MYSQL_HOST="${MYSQL_HOST:-$MYSQL_CONTAINER_NAME}" \
        -e MYSQL_ROOT_PASSWORD="$MYSQL_ROOT_PASSWORD" \
        -e USER_PG_PASSWORD="$USER_PG_PASSWORD" \
        -e USER_PG_HOST="${USER_PG_HOST:-$POSTGRES_CONTAINER_NAME}" \
        -e USER_PG_PORT="${USER_PG_PORT:-5432}" \
        -e APP_ENV="${APP_ENV:-production}" \
        -e TRUSTED_PROXY_CIDRS="${TRUSTED_PROXY_CIDRS:-}" \
        -e DOCKER_NETWORK=paas-network \
        -e NGINX_WEBHOOK_ENABLED="${NGINX_WEBHOOK_ENABLED:-false}" \
        -e NGINX_WEBHOOK_URL="$NGINX_WEBHOOK_URL" \
        -e NGINX_WEBHOOK_KEY="$NGINX_WEBHOOK_KEY" \
        -e INTERNAL_IP="${INTERNAL_IP:-127.0.0.1}" \
        -e GITHUB_APP_ID="${GITHUB_APP_ID:-}" \
        -e GITHUB_APP_PRIVATE_KEY_PATH="${GITHUB_APP_PRIVATE_KEY_PATH:-}" \
        -e GITHUB_APP_WEBHOOK_SECRET="${GITHUB_APP_WEBHOOK_SECRET:-}" \
        paas-worker:latest
}

stop_worker() {
    echo -e "${YELLOW}Stopping Worker Manager (paas-worker-manager)...${NC}"
    docker stop paas-worker-manager 2>/dev/null || true
    docker rm paas-worker-manager 2>/dev/null || true
}

start_frontend() {
    echo -e "${YELLOW}Starting Frontend (paas-frontend)...${NC}"
    prepare_env
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

stop_frontend() {
    echo -e "${YELLOW}Stopping Frontend (paas-frontend)...${NC}"
    docker stop paas-frontend 2>/dev/null || true
    docker rm paas-frontend 2>/dev/null || true
}

start_all() {
    start_mysql
    start_postgres
    start_user_postgres
    start_redis
    start_traefik
    start_backend
    start_worker
    start_frontend
    echo -e "${GREEN}[SUCCESS] All services started!${NC}"
}

stop_all() {
    echo -e "${YELLOW}Stopping all platform services...${NC}"
    docker ps -a --format '{{.Names}}' | grep '^paas-worker-s' | xargs -r docker rm -f 2>/dev/null || true
    stop_frontend
    stop_backend
    stop_worker
    stop_traefik
    stop_redis
    stop_user_postgres
    stop_postgres
    stop_mysql
    echo -e "${GREEN}[SUCCESS] All containers stopped${NC}"
}

# 3. Diagnostic & State Display
show_status() {
    echo -e "\n${GREEN}=== Runara Container Status ===${NC}"
    echo -e "------------------------------------------------------------"
    printf " %-22s | %-18s | %-15s\n" "Service Name" "Status" "IP Address"
    echo -e "------------------------------------------------------------"
    local services=("$MYSQL_CONTAINER_NAME" "paas-postgres" "$POSTGRES_CONTAINER_NAME" "paas-redis" "paas-traefik" "paas-backend" "paas-worker-manager" "paas-frontend")
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

# 4. Service Actions Dispatchers
start_service() {
    case "$1" in
        mysql) start_mysql ;;
        postgres|psql) start_postgres ;;
        user-postgres) start_user_postgres ;;
        redis) start_redis ;;
        traefik) start_traefik ;;
        backend) start_backend ;;
        worker) start_worker ;;
        frontend) start_frontend ;;
        *) echo -e "${RED}Unknown service: $1${NC}" ;;
    esac
}

stop_service() {
    case "$1" in
        mysql) stop_mysql ;;
        postgres|psql) stop_postgres ;;
        user-postgres) stop_user_postgres ;;
        redis) stop_redis ;;
        traefik) stop_traefik ;;
        backend) stop_backend ;;
        worker) stop_worker ;;
        frontend) stop_frontend ;;
        *) echo -e "${RED}Unknown service: $1${NC}" ;;
    esac
}

restart_service() {
    stop_service "$1"
    start_service "$1"
}

# 5. Interactive UI Menus
service_menu() {
    while true; do
        echo -e "\n${YELLOW}=== Manage Individual Service ===${NC}"
        echo "1) MySQL ($MYSQL_CONTAINER_NAME)"
        echo "2) PostgreSQL (paas-postgres)"
        echo "3) User PostgreSQL ($POSTGRES_CONTAINER_NAME)"
        echo "4) Redis (paas-redis)"
        echo "5) Traefik (paas-traefik)"
        echo "6) Backend (paas-backend)"
        echo "7) Worker Manager (paas-worker-manager)"
        echo "8) Frontend (paas-frontend)"
        echo "9) Back to Main Menu"
        read -p "Select service [1-9]: " s_opt

        local svc=""
        case "$s_opt" in
            1) svc="mysql" ;;
            2) svc="postgres" ;;
            3) svc="user-postgres" ;;
            4) svc="redis" ;;
            5) svc="traefik" ;;
            6) svc="backend" ;;
            7) svc="worker" ;;
            8) svc="frontend" ;;
            9) return 0 ;;
            *) echo -e "${RED}Invalid option!${NC}" && continue ;;
        esac

        while true; do
            echo -e "\n${YELLOW}=== Action for $svc ===${NC}"
            echo "1) Start"
            echo "2) Stop"
            echo "3) Restart"
            echo "4) Back to Service List"
            read -p "Select action [1-4]: " a_opt

            case "$a_opt" in
                1) start_service "$svc" && break ;;
                2) stop_service "$svc" && break ;;
                3) restart_service "$svc" && break ;;
                4) break ;;
                *) echo -e "${RED}Invalid action!${NC}" ;;
            esac
        done
    done
}

interactive_menu() {
    while true; do
        echo -e "${YELLOW}=== Runara Control Panel ===${NC}"
        echo "1) Start/Restart All Services"
        echo "2) Stop All Services"
        echo "3) Manage Individual Service"
        echo "4) Show Container Status"
        echo "5) Exit"
        read -p "Select option [1-5]: " main_opt

        case "$main_opt" in
            1) start_all ;;
            2) stop_all ;;
            3) service_menu ;;
            4) show_status ;;
            5|q|Q) echo "Exiting control panel." && exit 0 ;;
            *) echo -e "${RED}Invalid option!${NC}\n" ;;
        esac
    done
}

# 6. Main Execution Parser
init_vars

case "$1" in
    start)
        if [ -z "$2" ] || [ "$2" == "all" ]; then
            start_all
        else
            start_service "$2"
        fi
        ;;
    stop)
        if [ -z "$2" ] || [ "$2" == "all" ]; then
            stop_all
        else
            stop_service "$2"
        fi
        ;;
    restart)
        if [ -z "$2" ] || [ "$2" == "all" ]; then
            stop_all
            start_all
        else
            restart_service "$2"
        fi
        ;;
    status)
        show_status
        ;;
    --clean)
        echo "[CLEAN] Removing platform containers..."
        docker rm -f paas-frontend paas-backend paas-worker-manager paas-traefik paas-redis "$MYSQL_CONTAINER_NAME" paas-postgres "$POSTGRES_CONTAINER_NAME" 2>/dev/null || true
        echo "[SUCCESS] Containers removed"
        ;;
    --purge)
        echo "[PURGE] Removing platform containers and volumes..."
        docker rm -f paas-frontend paas-backend paas-worker-manager paas-traefik paas-redis "$MYSQL_CONTAINER_NAME" paas-postgres "$POSTGRES_CONTAINER_NAME" 2>/dev/null || true
        docker volume rm paas-redis-data paas-letsencrypt 2>/dev/null || true
        echo "[SUCCESS] Containers removed; Redis and TLS volumes purged (MySQL & PG data preserved)"
        ;;
    "")
        interactive_menu
        ;;
    *)
        echo "Usage: $0 [start|stop|restart|status] [service_name]"
        echo "       $0 [--clean|--purge]"
        echo "Services: mysql, postgres/psql, user-postgres, redis, traefik, backend, worker, frontend"
        exit 1
        ;;
esac
