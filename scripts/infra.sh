#!/bin/bash

# 1. Load environment variables dengan cara yang lebih bersih
if [ -f .env ]; then
    echo "[INFO] Loading .env file..."
    # Membaca .env tanpa memunculkan semua variabel sistem
    export $(grep -v '^#' .env | xargs -d '\n')
else
    echo "[ERROR] .env file not found! Copy .env.example to .env first."
    exit 1
fi

# Set Defaults
MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PASSWORD:-"rootpassword"}
MYSQL_DATABASE=${MYSQL_DATABASE:-"paas"}

# 2. Bersihkan kontainer lama
echo "[INFO] Cleaning old infrastructure..."
docker rm -f paas-mysql paas-postgres paas-redis paas-traefik paas-buildkit paas-registry 2>/dev/null || true

# 3. Siapkan Network & Folder (Pakai sudo untuk folder agar aman dari Permission Denied)
echo "[INFO] Preparing storage folders..."
docker network create paas-network 2>/dev/null || true
sudo mkdir -p storage/mysql storage/postgres storage/projects
sudo chown -R $(id -u):$(id -g) storage/  # Ubah kepemilikan ke user saat ini

# 4. Jalankan MariaDB
echo "[INFO] Starting MariaDB..."
docker run -d \
    --name paas-mysql \
    --network paas-network \
    --restart unless-stopped \
    -e MYSQL_ROOT_PASSWORD="$MYSQL_ROOT_PASSWORD" \
    -e MYSQL_DATABASE="$MYSQL_DATABASE" \
    -p 3306:3306 \
    -v "$(pwd)/storage/mysql:/var/lib/mysql" \
    mariadb:10.11

# 5. Jalankan PostgreSQL
echo "[INFO] Starting PostgreSQL..."
docker run -d \
    --name paas-postgres \
    --network paas-network \
    --restart unless-stopped \
    -e POSTGRES_USER="$PG_USER" \
    -e POSTGRES_PASSWORD="$PG_PASSWORD" \
    -e POSTGRES_DB="$PG_DATABASE" \
    -p 5432:5432 \
    -v "$(pwd)/storage/postgres:/var/lib/postgresql/data" \
    postgres:15-alpine

# 6. Jalankan Redis
echo "[INFO] Starting Redis..."
REDIS_CMD=""
[ ! -z "$REDIS_PASSWORD" ] && REDIS_CMD="redis-server --requirepass $REDIS_PASSWORD"
docker run -d \
    --name paas-redis \
    --network paas-network \
    --restart unless-stopped \
    -p 6379:6379 \
    redis:alpine $REDIS_CMD

# 7. Jalankan Traefik
echo "[INFO] Starting Traefik..."
docker run -d \
    --name paas-traefik \
    --network paas-network \
    --restart unless-stopped \
    -p 80:80 \
    -p 443:443 \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    -v "$(pwd)/docker/traefik/traefik.yml:/traefik.yml:ro" \
    traefik:v3.6

# 7.5. Jalankan Registry
echo "[INFO] Starting Local Registry..."
REGISTRY_PORT=${REGISTRY_PORT:-"5000"}
REGISTRY_HOST=${REGISTRY_HOST:-"127.0.0.1"}
REGISTRY_IMAGE=${REGISTRY_IMAGE:-"registry:2"}

docker volume create paas-registry-data 2>/dev/null || true
docker run -d \
    --name paas-registry \
    --network paas-network \
    -p "${REGISTRY_HOST}:${REGISTRY_PORT}:5000" \
    --restart unless-stopped \
    -v paas-registry-data:/var/lib/registry \
    "${REGISTRY_IMAGE}"

# 8. Jalankan BuildKit
echo "[INFO] Starting BuildKit..."
docker volume create paas-buildkit-cache 2>/dev/null || true
docker run -d \
    --name paas-buildkit \
    --network paas-network \
    -p 127.0.0.1:1234:1234 \
    --privileged \
    --restart unless-stopped \
    --cpus="2.0" \
    --memory="3g" \
    -v paas-buildkit-cache:/var/lib/buildkit \
    -v "$(pwd)/docker/templates/buildkitd.toml:/etc/buildkit/buildkitd.toml:ro" \
    moby/buildkit:rootless --addr tcp://0.0.0.0:1234 --config /etc/buildkit/buildkitd.toml

echo "[SUCCESS] Infrastructure is up! Cek status dengan: docker ps"
