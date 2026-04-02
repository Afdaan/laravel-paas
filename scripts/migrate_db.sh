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

# 3. Prepare URIs using getenv for pgloader (Safe for special characters)
# These will be read by pgloader inside the container from the environment
SOURCE_URI="mysql://getenv(\"M_USER\"):getenv(\"M_PASS\")@paas-mysql:3306/getenv(\"M_DB\")"
TARGET_URI="postgresql://getenv(\"P_USER\"):getenv(\"P_PASS\")@paas-postgres:5432/getenv(\"P_DB\")"

# 4. Generate dynamic pgloader command file
LOAD_FILE="${PROJECT_ROOT}/scripts/migration.load"

echo "[INFO] Generating temporary load configuration..."
cat <<EOF > "$LOAD_FILE"
LOAD DATABASE
     FROM $SOURCE_URI
     INTO $TARGET_URI

 ALTER SCHEMA '$MYSQL_DATABASE' RENAME TO 'public'

 CAST type tinyint to boolean drop typemod;
EOF

# 5. Execute pgloader
echo "[INFO] Running pgloader ETL process..."
echo "[INFO] Source (MySQL): $MYSQL_USER@paas-mysql:$MYSQL_DATABASE"
echo "[INFO] Target (PostgreSQL): $PG_USER@paas-postgres:$PG_DATABASE"

docker run --rm -i \
    --network paas-network \
    -e M_USER="$MYSQL_USER" \
    -e M_PASS="$MYSQL_PASSWORD" \
    -e M_DB="$MYSQL_DATABASE" \
    -e P_USER="$PG_USER" \
    -e P_PASS="$PG_PASSWORD" \
    -e P_DB="$PG_DATABASE" \
    -v "${LOAD_FILE}:/tmp/migration.load:ro" \
    dimitri/pgloader:latest pgloader /tmp/migration.load

EXIT_CODE=$?

# Cleanup
rm -f "$LOAD_FILE"

if [ $EXIT_CODE -eq 0 ]; then
    echo "[SUCCESS] Data migration completed successfully."
    echo "[INFO] Next step: Restart backend to apply PostgreSQL configuration."
else
    echo "[ERROR] Migration encountered an issue (Exit Code: $EXIT_CODE)."
fi
