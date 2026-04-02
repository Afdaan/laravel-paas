#!/bin/bash
set -e

# ===========================================
# MySQL to PostgreSQL Database Migrator
# ===========================================
# Migrates schema and data using the official pgloader
# ===========================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "[INFO] Starting database migration..."

# 1. Load credentials
if [ -f "$PROJECT_ROOT/.env" ]; then
    echo "[INFO] Loading .env credentials..."
    set -a
    source "$PROJECT_ROOT/.env"
    set +a
fi

MYSQL_USER=${MYSQL_USER:-"paas"}
MYSQL_PASSWORD=${MYSQL_PASSWORD:-""}
MYSQL_DATABASE=${MYSQL_DATABASE:-"paas"}

PG_USER=${PG_USER:-"paas"}
PG_PASSWORD=${PG_PASSWORD:-""}
PG_DATABASE=${PG_DATABASE:-"paas"}

# 2. Pre-flight Checks
if ! docker ps --format '{{.Names}}' | grep -q "^paas-mysql$"; then
    echo "[ERROR] paas-mysql container is not running. Start infrastructure first."
    exit 1
fi

if ! docker ps --format '{{.Names}}' | grep -q "^paas-postgres$"; then
    echo "[ERROR] paas-postgres container is not running. Start infrastructure first."
    exit 1
fi

echo "[INFO] Both databases are running. Commencing data transfer..."

# 3. Handle Special Characters in Passwords for URI Scheme
# We use simple string replacement to URL encode standard symbols safely for pgloader parsing
ENCODED_MYSQL_PASS=$(echo "$MYSQL_PASSWORD" | sed -e 's/#/%23/g' -e 's/@/%40/g' -e 's/:/%3A/g')
ENCODED_PG_PASS=$(echo "$PG_PASSWORD" | sed -e 's/#/%23/g' -e 's/@/%40/g' -e 's/:/%3A/g')

MYSQL_URI="mysql://${MYSQL_USER}:${ENCODED_MYSQL_PASS}@paas-mysql:3306/${MYSQL_DATABASE}"
PG_URI="postgresql://${PG_USER}:${ENCODED_PG_PASS}@paas-postgres:5432/${PG_DATABASE}"

# 4. Execute pgloader
# - We use 'dimitri/pgloader' via Docker to avoid host dependencies
# - We cast tinyint to boolean explicitly, as GORM in MySQL often uses tinyint(1) for bool
echo "[INFO] Running pgloader ETL process..."
docker run --rm -i --network paas-network dimitri/pgloader:latest pgloader \
   --cast "type tinyint to boolean drop typemod" \
   "$MYSQL_URI" \
   "$PG_URI"

EXIT_CODE=$?

if [ $EXIT_CODE -eq 0 ]; then
    echo "[SUCCESS] Data migration completed successfully."
    echo "[INFO] Next step: Restart backend to apply PostgreSQL configuration."
else
    echo "[ERROR] Migration encountered an issue (Exit Code: $EXIT_CODE)."
fi
