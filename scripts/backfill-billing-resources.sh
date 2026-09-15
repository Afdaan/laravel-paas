#!/bin/bash
set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Set current directory to project root
cd "$(dirname "$0")/.."

MODE="dry-run"
FORCE_AMBIGUOUS=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --apply)
            MODE="apply"
            shift
            ;;
        --dry-run)
            MODE="dry-run"
            shift
            ;;
        --force-default-for-ambiguous)
            FORCE_AMBIGUOUS=true
            shift
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            echo "Usage: $0 [--dry-run | --apply] [--force-default-for-ambiguous]"
            exit 1
            ;;
    esac
done

echo -e "${BLUE}=====================================================${NC}"
echo -e "${BLUE}   Runara - Authoritative Billing Backfill & Dry-Run ${NC}"
echo -e "${BLUE}=====================================================${NC}"
echo -e "Mode: ${BOLD}${MODE}${NC}"
echo ""

if ! command -v docker &> /dev/null; then
    echo -e "${RED}Error: docker is required but not installed.${NC}"
    exit 1
fi

PG_CONTAINER_NAME="paas-postgres"
DB_USER="paas"
DB_NAME="paas"

if [ -f ".env" ]; then
    echo -e "${BLUE}Loading credentials from .env...${NC}"
    ENV_CONTAINER=$(grep "^PG_CONTAINER_NAME=" .env | cut -d '=' -f2)
    ENV_USER=$(grep "^PG_USER=" .env | cut -d '=' -f2)
    ENV_NAME=$(grep "^PG_DATABASE=" .env | cut -d '=' -f2)

    if [ -n "$ENV_CONTAINER" ]; then PG_CONTAINER_NAME=$ENV_CONTAINER; fi
    if [ -n "$ENV_USER" ]; then DB_USER=$ENV_USER; fi
    if [ -n "$ENV_NAME" ]; then DB_NAME=$ENV_NAME; fi
fi

echo -e "${YELLOW}Container: ${PG_CONTAINER_NAME}${NC}"
echo -e "${YELLOW}Database:  ${DB_NAME}${NC}"
echo -e "${YELLOW}User:      ${DB_USER}${NC}"
echo ""

# Analyzing unmapped resources (Read-Only query, does not modify database)
PROPOSED_OUTPUT=$(docker exec -i "$PG_CONTAINER_NAME" psql -v ON_ERROR_STOP=1 -X -U "$DB_USER" -d "$DB_NAME" -A -F '|' << 'EOF'
SELECT
    'project' AS r_type,
    p.id AS r_id,
    p.name AS r_name,
    COALESCE(p.cpu_limit::text, 'NULL') AS r_cpu,
    COALESCE(p.memory_limit, 'NULL') AS r_mem,
    CASE
        WHEN (p.cpu_limit IS NULL OR p.cpu_limit <= 0.5)
             AND (p.memory_limit IS NULL OR p.memory_limit IN ('1024MB', '1024', '1G', '1GB', '512MB', '256MB'))
        THEN 'small'
        WHEN (p.cpu_limit IS NULL OR p.cpu_limit <= 1.0)
             AND (p.memory_limit IN ('2048MB', '2048', '2G', '2GB'))
        THEN 'medium'
        WHEN p.cpu_limit > 0.5 AND p.cpu_limit <= 1.0
             AND (p.memory_limit IS NULL OR p.memory_limit IN ('1024MB', '1024', '1G', '1GB', '2048MB', '2048', '2G', '2GB'))
        THEN 'medium'
        WHEN (p.cpu_limit IS NULL OR p.cpu_limit <= 2.0)
             AND (p.memory_limit IN ('4096MB', '4096', '4G', '4GB'))
        THEN 'large'
        WHEN p.cpu_limit > 1.0 AND p.cpu_limit <= 2.0
             AND (p.memory_limit IS NULL OR p.memory_limit IN ('1024MB', '1024', '1G', '1GB', '2048MB', '2048', '2G', '2GB', '4096MB', '4096', '4G', '4GB'))
        THEN 'large'
        ELSE 'AMBIGUOUS'
    END AS derived_slug,
    CASE
        WHEN (p.cpu_limit IS NULL AND p.memory_limit IS NULL) THEN 'Default platform allocation'
        WHEN (p.cpu_limit IS NOT NULL OR p.memory_limit IS NOT NULL) THEN 'Matched CPU/Memory limits'
        ELSE 'Custom unmapped allocation'
    END AS reason
FROM projects p
WHERE p.status <> 'deleting'
  AND NOT EXISTS (SELECT 1 FROM billable_resources br WHERE br.type = 'project' AND br.resource_id = p.id)

UNION ALL

SELECT
    'database' AS r_type,
    d.id AS r_id,
    d.name AS r_name,
    'conns:' || d.connection_limit AS r_cpu,
    'storage:' || d.storage_allocation AS r_mem,
    CASE
        WHEN d.connection_limit <= 50 THEN 'small'
        WHEN d.connection_limit <= 100 THEN 'medium'
        WHEN d.connection_limit <= 200 THEN 'large'
        ELSE 'AMBIGUOUS'
    END AS derived_slug,
    CASE
        WHEN d.connection_limit <= 50 THEN 'Default/Small connection limit (<=50)'
        WHEN d.connection_limit <= 100 THEN 'Medium connection limit (<=100)'
        WHEN d.connection_limit <= 200 THEN 'Large connection limit (<=200)'
        ELSE 'Custom connection limit (>200)'
    END AS reason
FROM database_instances d
WHERE d.status <> 'deleted'
  AND NOT EXISTS (SELECT 1 FROM billable_resources br WHERE br.type = 'database' AND br.resource_id = d.id);
EOF
)

TOTAL_CANDIDATES=0
AMBIGUOUS_COUNT=0

printf "%-10s | %-6s | %-25s | %-12s | %-16s | %-10s | %-35s\n" "TYPE" "ID" "NAME" "CPU/METRIC" "MEMORY/STORAGE" "SPEC" "REASON"
echo "---------------------------------------------------------------------------------------------------------------------------------"

while IFS='|' read -r r_type r_id r_name r_cpu r_mem derived_slug reason; do
    # Skip header and footer lines
    if [ "$r_type" = "r_type" ] || [[ "$r_type" == *"row"* ]] || [ -z "$r_type" ]; then
        continue
    fi
    TOTAL_CANDIDATES=$((TOTAL_CANDIDATES + 1))
    if [ "$derived_slug" = "AMBIGUOUS" ]; then
        AMBIGUOUS_COUNT=$((AMBIGUOUS_COUNT + 1))
        printf "${RED}%-10s | %-6s | %-25s | %-12s | %-16s | %-10s | %-35s${NC}\n" "$r_type" "$r_id" "$r_name" "$r_cpu" "$r_mem" "$derived_slug" "$reason"
    else
        printf "%-10s | %-6s | %-25s | %-12s | %-16s | ${GREEN}%-10s${NC} | %-35s\n" "$r_type" "$r_id" "$r_name" "$r_cpu" "$r_mem" "$derived_slug" "$reason"
    fi
done <<< "$PROPOSED_OUTPUT"

echo "---------------------------------------------------------------------------------------------------------------------------------"
echo -e "Total unmapped resources found: ${BOLD}${TOTAL_CANDIDATES}${NC}"
if [ $AMBIGUOUS_COUNT -gt 0 ]; then
    echo -e "${RED}Ambiguous resources detected: ${AMBIGUOUS_COUNT}${NC}"
fi
echo ""

if [ $TOTAL_CANDIDATES -eq 0 ]; then
    echo -e "${GREEN}✓ All active projects and databases already have billable spec mappings.${NC}"
    exit 0
fi

if [ $AMBIGUOUS_COUNT -gt 0 ] && [ "$FORCE_AMBIGUOUS" = false ]; then
    echo -e "${RED}[ERROR] Migration blocked: $AMBIGUOUS_COUNT resources have ambiguous limits that do not match standard Small/Medium/Large specs.${NC}"
    echo -e "${YELLOW}Please inspect the ambiguous rows listed above, adjust limits, or run with --force-default-for-ambiguous to force Small mapping on ambiguous rows only.${NC}"
    exit 1
fi

if [ "$MODE" = "dry-run" ]; then
    echo -e "${YELLOW}[DRY-RUN] No database modifications were performed. Re-run with '--apply' to commit this mapping.${NC}"
    exit 0
fi

echo -e "${BLUE}Applying billable resources backfill in transaction...${NC}"

docker exec -i "$PG_CONTAINER_NAME" psql -v ON_ERROR_STOP=1 -X -U "$DB_USER" -d "$DB_NAME" << EOF
BEGIN;

-- 1. Ensure active billable specs exist
INSERT INTO billable_specs (type, name, slug, cpu_millicores, memory_mb, storage_gb, monthly_credits, connection_limit, backup_retention_days, version, is_active, created_at, updated_at)
VALUES
('project', 'Small', 'small', 500, 1024, 5, 100, NULL, NULL, 1, true, NOW(), NOW()),
('project', 'Medium', 'medium', 1000, 2048, 10, 200, NULL, NULL, 1, true, NOW(), NOW()),
('project', 'Large', 'large', 2000, 4096, 20, 400, NULL, NULL, 1, true, NOW(), NOW()),
('database', 'Small', 'small', 500, 1024, 10, 150, 50, 7, 1, true, NOW(), NOW()),
('database', 'Medium', 'medium', 1000, 2048, 25, 300, 100, 14, 1, true, NOW(), NOW()),
('database', 'Large', 'large', 2000, 4096, 50, 600, 200, 30, 1, true, NOW(), NOW())
ON CONFLICT DO NOTHING;

-- 2. Backfill projects matching Large
INSERT INTO billable_resources (user_id, type, resource_id, spec_id, billing_status, current_period_start, next_invoice_at, billing_anchor_day, billing_anchor_month_end, created_at, updated_at)
SELECT
    p.user_id,
    'project',
    p.id,
    (SELECT id FROM billable_specs WHERE type = 'project' AND slug = 'large' AND is_active = true ORDER BY version DESC LIMIT 1),
    'active',
    NOW(),
    NOW() + INTERVAL '1 month',
    EXTRACT(DAY FROM NOW())::int,
    false,
    NOW(),
    NOW()
FROM projects p
WHERE p.status <> 'deleting'
  AND NOT EXISTS (SELECT 1 FROM billable_resources br WHERE br.type = 'project' AND br.resource_id = p.id)
  AND (
      ((p.cpu_limit IS NULL OR p.cpu_limit <= 2.0) AND (p.memory_limit IN ('4096MB', '4096', '4G', '4GB')))
      OR (p.cpu_limit > 1.0 AND p.cpu_limit <= 2.0 AND (p.memory_limit IS NULL OR p.memory_limit IN ('1024MB', '1024', '1G', '1GB', '2048MB', '2048', '2G', '2GB', '4096MB', '4096', '4G', '4GB')))
  );

-- 3. Backfill projects matching Medium
INSERT INTO billable_resources (user_id, type, resource_id, spec_id, billing_status, current_period_start, next_invoice_at, billing_anchor_day, billing_anchor_month_end, created_at, updated_at)
SELECT
    p.user_id,
    'project',
    p.id,
    (SELECT id FROM billable_specs WHERE type = 'project' AND slug = 'medium' AND is_active = true ORDER BY version DESC LIMIT 1),
    'active',
    NOW(),
    NOW() + INTERVAL '1 month',
    EXTRACT(DAY FROM NOW())::int,
    false,
    NOW(),
    NOW()
FROM projects p
WHERE p.status <> 'deleting'
  AND NOT EXISTS (SELECT 1 FROM billable_resources br WHERE br.type = 'project' AND br.resource_id = p.id)
  AND (
      ((p.cpu_limit IS NULL OR p.cpu_limit <= 1.0) AND (p.memory_limit IN ('2048MB', '2048', '2G', '2GB')))
      OR (p.cpu_limit > 0.5 AND p.cpu_limit <= 1.0 AND (p.memory_limit IS NULL OR p.memory_limit IN ('1024MB', '1024', '1G', '1GB', '2048MB', '2048', '2G', '2GB')))
  );

-- 4. Backfill projects matching Small
INSERT INTO billable_resources (user_id, type, resource_id, spec_id, billing_status, current_period_start, next_invoice_at, billing_anchor_day, billing_anchor_month_end, created_at, updated_at)
SELECT
    p.user_id,
    'project',
    p.id,
    (SELECT id FROM billable_specs WHERE type = 'project' AND slug = 'small' AND is_active = true ORDER BY version DESC LIMIT 1),
    'active',
    NOW(),
    NOW() + INTERVAL '1 month',
    EXTRACT(DAY FROM NOW())::int,
    false,
    NOW(),
    NOW()
FROM projects p
WHERE p.status <> 'deleting'
  AND NOT EXISTS (SELECT 1 FROM billable_resources br WHERE br.type = 'project' AND br.resource_id = p.id)
  AND ((p.cpu_limit IS NULL OR p.cpu_limit <= 0.5) AND (p.memory_limit IS NULL OR p.memory_limit IN ('1024MB', '1024', '1G', '1GB', '512MB', '256MB')));

-- 5. If --force-default-for-ambiguous is specified, map any remaining unmapped ambiguous projects to Small
$([ "$FORCE_AMBIGUOUS" = true ] && cat << 'FORCE_PROJECTS'
INSERT INTO billable_resources (user_id, type, resource_id, spec_id, billing_status, current_period_start, next_invoice_at, billing_anchor_day, billing_anchor_month_end, created_at, updated_at)
SELECT
    p.user_id,
    'project',
    p.id,
    (SELECT id FROM billable_specs WHERE type = 'project' AND slug = 'small' AND is_active = true ORDER BY version DESC LIMIT 1),
    'active',
    NOW(),
    NOW() + INTERVAL '1 month',
    EXTRACT(DAY FROM NOW())::int,
    false,
    NOW(),
    NOW()
FROM projects p
WHERE p.status <> 'deleting'
  AND NOT EXISTS (SELECT 1 FROM billable_resources br WHERE br.type = 'project' AND br.resource_id = p.id);
FORCE_PROJECTS
)

-- 6. Backfill databases matching Large
INSERT INTO billable_resources (user_id, type, resource_id, spec_id, billing_status, current_period_start, next_invoice_at, billing_anchor_day, billing_anchor_month_end, created_at, updated_at)
SELECT
    d.user_id,
    'database',
    d.id,
    (SELECT id FROM billable_specs WHERE type = 'database' AND slug = 'large' AND is_active = true ORDER BY version DESC LIMIT 1),
    'active',
    NOW(),
    NOW() + INTERVAL '1 month',
    EXTRACT(DAY FROM NOW())::int,
    false,
    NOW(),
    NOW()
FROM database_instances d
WHERE d.status <> 'deleted'
  AND NOT EXISTS (SELECT 1 FROM billable_resources br WHERE br.type = 'database' AND br.resource_id = d.id)
  AND d.connection_limit > 100 AND d.connection_limit <= 200;

-- 7. Backfill databases matching Medium
INSERT INTO billable_resources (user_id, type, resource_id, spec_id, billing_status, current_period_start, next_invoice_at, billing_anchor_day, billing_anchor_month_end, created_at, updated_at)
SELECT
    d.user_id,
    'database',
    d.id,
    (SELECT id FROM billable_specs WHERE type = 'database' AND slug = 'medium' AND is_active = true ORDER BY version DESC LIMIT 1),
    'active',
    NOW(),
    NOW() + INTERVAL '1 month',
    EXTRACT(DAY FROM NOW())::int,
    false,
    NOW(),
    NOW()
FROM database_instances d
WHERE d.status <> 'deleted'
  AND NOT EXISTS (SELECT 1 FROM billable_resources br WHERE br.type = 'database' AND br.resource_id = d.id)
  AND d.connection_limit > 50 AND d.connection_limit <= 100;

-- 8. Backfill databases matching Small
INSERT INTO billable_resources (user_id, type, resource_id, spec_id, billing_status, current_period_start, next_invoice_at, billing_anchor_day, billing_anchor_month_end, created_at, updated_at)
SELECT
    d.user_id,
    'database',
    d.id,
    (SELECT id FROM billable_specs WHERE type = 'database' AND slug = 'small' AND is_active = true ORDER BY version DESC LIMIT 1),
    'active',
    NOW(),
    NOW() + INTERVAL '1 month',
    EXTRACT(DAY FROM NOW())::int,
    false,
    NOW(),
    NOW()
FROM database_instances d
WHERE d.status <> 'deleted'
  AND NOT EXISTS (SELECT 1 FROM billable_resources br WHERE br.type = 'database' AND br.resource_id = d.id)
  AND d.connection_limit <= 50;

-- 9. If --force-default-for-ambiguous is specified, map any remaining unmapped ambiguous databases to Small
$([ "$FORCE_AMBIGUOUS" = true ] && cat << 'FORCE_DATABASES'
INSERT INTO billable_resources (user_id, type, resource_id, spec_id, billing_status, current_period_start, next_invoice_at, billing_anchor_day, billing_anchor_month_end, created_at, updated_at)
SELECT
    d.user_id,
    'database',
    d.id,
    (SELECT id FROM billable_specs WHERE type = 'database' AND slug = 'small' AND is_active = true ORDER BY version DESC LIMIT 1),
    'active',
    NOW(),
    NOW() + INTERVAL '1 month',
    EXTRACT(DAY FROM NOW())::int,
    false,
    NOW(),
    NOW()
FROM database_instances d
WHERE d.status <> 'deleted'
  AND NOT EXISTS (SELECT 1 FROM billable_resources br WHERE br.type = 'database' AND br.resource_id = d.id);
FORCE_DATABASES
)

COMMIT;
EOF

echo ""
echo -e "${GREEN}✓ Billable resources backfill applied successfully!${NC}"
echo -e "${BLUE}You can now restart your backend container or redeploy.${NC}"
