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

# Use the application user for migration (as it has global '%' access by default)
MYSQL_USER=${MYSQL_USER:-"paas"}
MYSQL_PASSWORD=${MYSQL_PASSWORD:-""}

if [ -z "$MYSQL_PASSWORD" ]; then
    echo -n "Enter MySQL password for user '$MYSQL_USER' at 'paas-mysql': "
    read -rs MYSQL_PASSWORD
    echo ""
fi

MYSQL_DATABASE=${MYSQL_DATABASE:-"paas"}

PG_USER=${PG_USER:-"paas"}
PG_PASSWORD=${PG_PASSWORD:-""}
PG_DATABASE=${PG_DATABASE:-"paas"}

# 2. Preparation
TEMP_DUMP="${PROJECT_ROOT}/scripts/mysql_dump.sql"

echo "[INFO] Commencing Hybrid Migration (Dump & Load)..."

# 3. Extracting data via docker exec (Bypassing network auth like create_admin.sh)
echo "[INFO] Step 1: Exporting data from MySQL via 'docker exec'..."
if [ ! -z "$MYSQL_ROOT_PASSWORD" ]; then
    M_USER="root"
    M_PASS="$MYSQL_ROOT_PASSWORD"
else
    M_USER="$MYSQL_USER"
    M_PASS="$MYSQL_PASSWORD"
fi

# We use mysqldump inside the container and capture it locally
docker exec -e MYSQL_PWD="$M_PASS" paas-mysql mysqldump -u"$M_USER" --compact --no-create-db "$MYSQL_DATABASE" > "$TEMP_DUMP"

if [ ! -s "$TEMP_DUMP" ]; then
    echo "[ERROR] MySQL dump failed or resulted in empty file. Check your MYSQL_ROOT_PASSWORD."
    rm -f "$TEMP_DUMP"
    exit 1
fi

# 4. Generate dynamic pgloader command file to process the SQL dump
LOAD_FILE="${PROJECT_ROOT}/scripts/migration.load"
PG_URI="postgresql://\${P_USER}:\${P_PASS}@paas-postgres:5432/\${P_DB}"

echo "[INFO] Step 2: Preparing PostgreSQL translation script..."
cat <<EOF > "$LOAD_FILE"
LOAD ARCHIVE
     FROM $TEMP_DUMP
     INTO $PG_URI

 WITH truncate, include drop, create tables, create indexes, reset sequences

 ALTER SCHEMA '$MYSQL_DATABASE' RENAME TO 'public'

 CAST type tinyint to boolean drop typemod;
EOF

# 5. Execute pgloader with the dump file
echo "[INFO] Step 3: Loading data into PostgreSQL..."
docker run --rm -i \
    --network paas-network \
    -e P_USER="$PG_USER" \
    -e P_PASS="$PG_PASSWORD" \
    -e P_DB="$PG_DATABASE" \
    -v "${PROJECT_ROOT}/scripts:/tmp/scripts:ro" \
    dimitri/pgloader:latest pgloader /tmp/scripts/migration.load

EXIT_CODE=$?

# Cleanup
rm -f "$TEMP_DUMP"
rm -f "$LOAD_FILE"

if [ $EXIT_CODE -eq 0 ]; then
    echo "[SUCCESS] Hybrid migration completed successfully."
else
    echo "[ERROR] Loading to PostgreSQL failed (Exit Code: $EXIT_CODE)."
fi
