#!/bin/bash
set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Set current directory to project root
cd "$(dirname "$0")/.."

echo -e "${BLUE}===========================================${NC}"
echo -e "${BLUE}   Runara - Backfill Billing Resources   ${NC}"
echo -e "${BLUE}===========================================${NC}"
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

echo -e "${BLUE}Running billable resources backfill...${NC}"

docker exec -i "$PG_CONTAINER_NAME" psql -U "$DB_USER" -d "$DB_NAME" << 'EOF'
-- Backfill unmapped projects
INSERT INTO billable_resources (user_id, type, resource_id, spec_id, billing_status, current_period_start, next_invoice_at, billing_anchor_day, billing_anchor_month_end, created_at, updated_at)
SELECT 
    user_id, 
    'project', 
    id, 
    (SELECT id FROM billable_specs WHERE type = 'project' AND is_active = true ORDER BY id ASC LIMIT 1), 
    'active', 
    NOW(), 
    NOW() + INTERVAL '1 month', 
    EXTRACT(DAY FROM NOW())::int, 
    false, 
    NOW(), 
    NOW()
FROM projects 
WHERE status <> 'deleting' 
  AND NOT EXISTS (SELECT 1 FROM billable_resources WHERE type = 'project' AND resource_id = projects.id);

-- Backfill unmapped database instances
INSERT INTO billable_resources (user_id, type, resource_id, spec_id, billing_status, current_period_start, next_invoice_at, billing_anchor_day, billing_anchor_month_end, created_at, updated_at)
SELECT 
    user_id, 
    'database', 
    id, 
    (SELECT id FROM billable_specs WHERE type = 'database' AND is_active = true ORDER BY id ASC LIMIT 1), 
    'active', 
    NOW(), 
    NOW() + INTERVAL '1 month', 
    EXTRACT(DAY FROM NOW())::int, 
    false, 
    NOW(), 
    NOW()
FROM database_instances 
WHERE status <> 'deleted' 
  AND NOT EXISTS (SELECT 1 FROM billable_resources WHERE type = 'database' AND resource_id = database_instances.id);
EOF

echo ""
echo -e "${GREEN}✓ Billable resources backfill completed successfully!${NC}"
echo -e "${BLUE}You can now restart your backend container or redeploy.${NC}"
