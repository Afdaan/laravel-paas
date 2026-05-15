#!/bin/bash
# ==============================================================================
# Laravel PaaS Runtime Builder
# This script builds the optimized Docker images for PHP runtime versions 8.0 to 8.4.
# Having these images pre-built on the server makes project creation and redeploy 
# lightning-fast (seconds instead of minutes) because it avoids repeating the installation 
# and compilation of PHP extensions.
# ==============================================================================

set -e

# Path logic
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
DOCKER_BASE="${PROJECT_ROOT}/docker/runtime/Dockerfile.base"
DOCKER_BUILDER="${PROJECT_ROOT}/docker/runtime/Dockerfile.builder"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}Building Laravel PaaS Runtime Images...${NC}"

# Versions to build
VERSIONS=("8.0" "8.1" "8.2" "8.3" "8.4")

for VERSION in "${VERSIONS[@]}"; do
    # 1. Build Base Runtime
    TAG_RUNTIME="paas-runtime-php:${VERSION}-alpine"
    echo -e "${YELLOW}Building PHP ${VERSION} runtime... ($TAG_RUNTIME)${NC}"

    docker build \
        --build-arg PHP_VERSION="${VERSION}" \
        -f "${DOCKER_BASE}" \
        -t "${TAG_RUNTIME}" \
        "${PROJECT_ROOT}/docker/runtime"

    echo -e "${GREEN}[SUCCESS] PHP ${VERSION} runtime built successfully.${NC}"

    # 2. Build Unified Builder
    TAG_BUILDER="paas-builder-base:${VERSION}-alpine"
    echo -e "${YELLOW}Building PHP ${VERSION} Unified Builder... ($TAG_BUILDER)${NC}"

    docker build \
        --build-arg PHP_VERSION="${VERSION}" \
        -f "${DOCKER_BUILDER}" \
        -t "${TAG_BUILDER}" \
        "${PROJECT_ROOT}/docker/runtime"

    echo -e "${GREEN}[SUCCESS] PHP ${VERSION} builder built successfully.${NC}"
done

echo -e "${BLUE}[INFO] All runtime and builder images are ready! Now project builds will be instant.${NC}"
echo -e "${BLUE}Project Dockerfiles can now use: FROM paas-runtime-php:8.x-alpine & FROM paas-builder-base:8.x-alpine${NC}"
