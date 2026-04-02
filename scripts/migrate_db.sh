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

# Use root for source (MySQL) migration if available for full schema access
if [ ! -z "$MYSQL_ROOT_PASSWORD" ]; then
    MYSQL_USER="root"
    MYSQL_PASSWORD="$MYSQL_ROOT_PASSWORD"
else
    MYSQL_USER=${MYSQL_USER:-"paas"}
    MYSQL_PASSWORD=${MYSQL_PASSWORD:-""}
fi

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

# 3. URL-encode passwords to safely embed in pgloader URIs
urlencode() {
    echo -n "$1" | python3 -c "import urllib.parse, sys; print(urllib.parse.quote(sys.stdin.read(), safe=''))"
}

ENCODED_MYSQL_PASS=$(urlencode "$MYSQL_PASSWORD")
ENCODED_PG_PASS=$(urlencode "$PG_PASSWORD")

# 4. Generate pgloader command file with credentials baked in
LOAD_FILE="${PROJECT_ROOT}/scripts/migration.load"

echo "[INFO] Generating temporary load configuration..."
cat > "$LOAD_FILE" <<PGLOAD
LOAD DATABASE
     FROM mysql://${MYSQL_USER}:${ENCODED_MYSQL_PASS}@paas-mysql:3306/${MYSQL_DATABASE}
     INTO postgresql://${PG_USER}:${ENCODED_PG_PASS}@paas-postgres:5432/${PG_DATABASE}

 ALTER SCHEMA '${MYSQL_DATABASE}' RENAME TO 'public'

 CAST type tinyint to boolean drop typemod;
PGLOAD

# 5. Execute pgloader
echo "[INFO] Running pgloader ETL process..."
echo "[INFO] Source (MySQL): ${MYSQL_USER}@paas-mysql:${MYSQL_DATABASE}"
echo "[INFO] Target (PostgreSQL): ${PG_USER}@paas-postgres:${PG_DATABASE}"

docker run --rm -i \
    --network paas-network \
    -v "${LOAD_FILE}:/tmp/migration.load:ro" \
    dimitri/pgloader:latest pgloader /tmp/migration.load

EXIT_CODE=$?

# Cleanup temporary load file
rm -f "$LOAD_FILE"

if [ $EXIT_CODE -eq 0 ]; then
    echo "[SUCCESS] Data migration completed successfully."
    echo "[INFO] Next step: Restart backend to apply PostgreSQL configuration."
else
    echo "[ERROR] Migration encountered an issue (Exit Code: $EXIT_CODE)."
fi
