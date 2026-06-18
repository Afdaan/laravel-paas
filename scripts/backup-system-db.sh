#!/usr/bin/env bash

# Prevent shell errors from continuing
set -euo pipefail

# Determine the directory of the script and go to the project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Load configuration from .env if present
if [ -f "${PROJECT_ROOT}/.env" ]; then
    # shellcheck disable=SC1090
    export $(grep -v '^#' "${PROJECT_ROOT}/.env" | xargs)
fi

# Set database details with defaults
DB_CONTAINER="${PG_CONTAINER_NAME:-paas-postgres}"
DB_USER="${PG_USER:-postgres}"
DB_NAME="${PG_DATABASE:-paas}"
DB_PASSWORD="${PG_PASSWORD:-change_this_secure_password}"
BACKUP_DIR="${PROJECT_ROOT}/storage/backups/system_db"

# Ensure backup directory exists
mkdir -p "${BACKUP_DIR}"

# Build filename with timestamp
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
BACKUP_FILE="${BACKUP_DIR}/system_db_${TIMESTAMP}.sql"

# Perform backup using docker exec pg_dump
echo "Starting PostgreSQL backup for system database..."
if ! docker exec -i "${DB_CONTAINER}" env PGPASSWORD="${DB_PASSWORD}" pg_dump -U "${DB_USER}" "${DB_NAME}" > "${BACKUP_FILE}"; then
    echo "ERROR: PostgreSQL pg_dump failed!" >&2
    exit 1
fi

# Compress the backup file to save space
gzip "${BACKUP_FILE}"
echo "✓ System database backup completed successfully: ${BACKUP_FILE}.gz"

# Retain only the last 7 days of backups to prevent disk bloat
find "${BACKUP_DIR}" -name "system_db_*.sql.gz" -mtime +7 -delete
echo "✓ Pruned system database backups older than 7 days."
