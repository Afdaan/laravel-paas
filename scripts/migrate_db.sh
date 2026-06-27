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
MYSQL_CONTAINER_NAME=${MYSQL_CONTAINER_NAME:-"paas-mysql"}
MYSQL_HOST=${MYSQL_HOST:-"$MYSQL_CONTAINER_NAME"}
MYSQL_PORT=${MYSQL_PORT:-"3306"}
PG_HOST=${PG_HOST:-"paas-postgres"}
PG_PORT=${PG_PORT:-"5432"}

if [ -z "$MYSQL_PASSWORD" ]; then
    echo -n "Enter MySQL password for user '$MYSQL_USER' at '$MYSQL_HOST': "
    read -rs MYSQL_PASSWORD
    echo ""
fi

MYSQL_DATABASE=${MYSQL_DATABASE:-"paas"}

PG_USER=${PG_USER:-"paas"}
PG_PASSWORD=${PG_PASSWORD:-""}
PG_DATABASE=${PG_DATABASE:-"paas"}

# 2. Setup Temporary Migrator User (The most robust way)
M_MIGRATOR_USER="paas_migrator"
M_MIGRATOR_PASS="paas_migrator_123"

echo "[INFO] Setting up temporary MySQL user for migration..."
# We use docker exec to create the user with native password plugin for pgloader compatibility
# This bypasses all network/host/password mismatch issues from .env
if [ ! -z "$MYSQL_ROOT_PASSWORD" ]; then
    M_ROOT_PASS="$MYSQL_ROOT_PASSWORD"
else
    echo -n "Please enter your MySQL root password: "
    read -rs M_ROOT_PASS
    echo ""
fi

docker exec -e MYSQL_PWD="$M_ROOT_PASS" "$MYSQL_CONTAINER_NAME" mysql -u"root" -e "
CREATE USER IF NOT EXISTS '$M_MIGRATOR_USER'@'%' IDENTIFIED BY '$M_MIGRATOR_PASS';
ALTER USER '$M_MIGRATOR_USER'@'%' IDENTIFIED VIA mysql_native_password USING PASSWORD('$M_MIGRATOR_PASS');
GRANT ALL PRIVILEGES ON $MYSQL_DATABASE.* TO '$M_MIGRATOR_USER'@'%';
FLUSH PRIVILEGES;
" 2>/dev/null || { echo "[ERROR] Failed to setup migrator user. Check root password."; exit 1; }

# 3. URL-encode PostgreSQL password
urlencode() {
    echo -n "$1" | python3 -c "import urllib.parse, sys; print(urllib.parse.quote(sys.stdin.read(), safe=''))"
}
ENCODED_PG_PASS=$(urlencode "$PG_PASSWORD")

# 4. Generate pgloader command file
LOAD_FILE="${PROJECT_ROOT}/scripts/migration.load"

echo "[INFO] Generating load configuration..."
cat <<EOF > "$LOAD_FILE"
LOAD DATABASE
     FROM mysql://$M_MIGRATOR_USER:$M_MIGRATOR_PASS@${MYSQL_HOST}:${MYSQL_PORT}/$MYSQL_DATABASE
     INTO postgresql://${PG_USER}:${ENCODED_PG_PASS}@${PG_HOST}:${PG_PORT}/${PG_DATABASE}

 ALTER SCHEMA '$MYSQL_DATABASE' RENAME TO 'public'

 CAST type bigint to bigint drop typemod,
      type int to integer drop typemod,
      type tinyint to boolean drop typemod;
EOF

# 5. Execute pgloader
echo "[INFO] Running pgloader ETL process..."
docker run --rm -i \
    --network paas-network \
    -v "${LOAD_FILE}:/tmp/migration.load:ro" \
    dimitri/pgloader:latest pgloader /tmp/migration.load

EXIT_CODE=$?

# 6. Cleanup
echo "[INFO] Cleaning up temporary migrator user..."
docker exec -e MYSQL_PWD="$M_ROOT_PASS" "$MYSQL_CONTAINER_NAME" mysql -u"root" -e "DROP USER '$M_MIGRATOR_USER'@'%';" 2>/dev/null
rm -f "$LOAD_FILE"

if [ $EXIT_CODE -eq 0 ]; then
    echo "[SUCCESS] Data migration completed successfully."
else
    echo "[ERROR] Migration failed (Exit Code: $EXIT_CODE)."
fi
