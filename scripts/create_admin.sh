#!/bin/bash

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Set current directory to project root
cd "$(dirname "$0")/.."

echo -e "${BLUE}===========================================${NC}"
echo -e "${BLUE}     Laravel PaaS - Create Admin User     ${NC}"
echo -e "${BLUE}===========================================${NC}"
echo ""

# Check requirements
if ! command -v docker &> /dev/null; then
    echo -e "${RED}Error: docker is required but not installed.${NC}"
    exit 1
fi

# Load DB credentials from .env or use defaults
DB_USER="paas"
DB_PASS=""
DB_NAME="paas"

if [ -f "backend/.env" ]; then
    echo -e "${BLUE}Loading credentials from backend/.env...${NC}"
    # Extract values from .env (simple grep)
    ENV_USER=$(grep "^MYSQL_USER=" backend/.env | cut -d '=' -f2)
    ENV_PASS=$(grep "^MYSQL_PASSWORD=" backend/.env | cut -d '=' -f2)
    ENV_NAME=$(grep "^MYSQL_DATABASE=" backend/.env | cut -d '=' -f2)
    
    if [ ! -z "$ENV_USER" ]; then DB_USER=$ENV_USER; fi
    if [ ! -z "$ENV_PASS" ]; then DB_PASS=$ENV_PASS; fi
    if [ ! -z "$ENV_NAME" ]; then DB_NAME=$ENV_NAME; fi
fi

# Override default password for root if creating using root (optional)
# But we usually use the configured user
echo -e "${YELLOW}Please enter details for the new Super Admin user:${NC}"
echo ""

# Interactive Input
read -p "Name: " ADMIN_NAME
while [[ -z "$ADMIN_NAME" ]]; do
    echo -e "${RED}Name cannot be empty.${NC}"
    read -p "Name: " ADMIN_NAME
done

read -p "Email: " ADMIN_EMAIL
while [[ -z "$ADMIN_EMAIL" ]]; do
    echo -e "${RED}Email cannot be empty.${NC}"
    read -p "Email: " ADMIN_EMAIL
done

# Password input (hidden)
while true; do
    read -s -p "Password: " ADMIN_PASSWORD
    echo ""
    read -s -p "Confirm Password: " ADMIN_PASSWORD_CONFIRM
    echo ""
    
    if [[ -z "$ADMIN_PASSWORD" ]]; then
        echo -e "${RED}Password cannot be empty.${NC}"
    elif [[ "$ADMIN_PASSWORD" != "$ADMIN_PASSWORD_CONFIRM" ]]; then
        echo -e "${RED}Passwords do not match. Please try again.${NC}"
    else
        break
    fi
done

echo ""
echo -e "${BLUE}Generating password hash...${NC}"

# Generate BCrypt Hash using ephemeral PHP container
# This ensures we get a valid hash without installing PHP on host
HASH=$(docker run --rm php:8.2-cli-alpine php -r "echo password_hash('$ADMIN_PASSWORD', PASSWORD_BCRYPT);")

if [[ -z "$HASH" ]]; then
    echo -e "${RED}Error: Failed to generate password hash.${NC}"
    exit 1
fi

echo -e "${BLUE}Inserting user into database...${NC}"

# Check if MySQL container is running
if ! docker ps | grep -q paas-mysql; then
    echo -e "${RED}Error: paas-mysql container is not running.${NC}"
    echo -e "Please start the services first: ./scripts/start.sh"
    exit 1
fi

# Prepare SQL Query
SQL="INSERT INTO users (name, email, password, role, created_at, updated_at) VALUES ('$ADMIN_NAME', '$ADMIN_EMAIL', '$HASH', 'superadmin', NOW(), NOW());"

# Execute Query
# We use docker exec to run mysql client inside the container
docker exec -i paas-mysql mysql -u"$DB_USER" -p"$DB_PASS" "$DB_NAME" -e "$SQL"

EXIT_CODE=$?

if [ $EXIT_CODE -eq 0 ]; then
    echo ""
    echo -e "${GREEN}✨ Admin user created successfully!${NC}"
    echo -e "Name:  $ADMIN_NAME"
    echo -e "Email: ${YELLOW}$ADMIN_EMAIL${NC}"
    echo -e "Role:  superadmin"
    echo ""
    echo -e "You can now login at the dashboard."
else
    echo ""
    echo -e "${RED}❌ Failed to insert user into database.${NC}"
    echo -e "Error Code: $EXIT_CODE"
    echo -e "Check if email '$ADMIN_EMAIL' already exists."
fi
