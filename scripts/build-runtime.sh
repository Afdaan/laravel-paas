#!/bin/bash
# ==============================================================================
# Laravel PaaS Runtime Builder
# This script builds the optimized Docker images for PHP runtime versions 8.0 to 8.4.
# Usage: ./build-runtime.sh [target] [--force]
# Targets:
#   all      - Build all images (default)
#   runtime  - Build only PHP runtime images
#   builder  - Build only Unified Builder images
#   [target]:[version] - Build specific version (e.g. runtime:8.2 or builder:8.4)
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
RED='\033[0;31m'
NC='\033[0m'

# Parse arguments
TARGET_ARG="all"
FORCE_REBUILD=false

for arg in "$@"; do
    if [[ "$arg" == "--force" ]]; then
        FORCE_REBUILD=true
    else
        TARGET_ARG="$arg"
    fi
done

if [ "$FORCE_REBUILD" = true ]; then
    echo -e "${YELLOW}Force rebuild enabled.${NC}"
fi

# Split target and version
IFS=':' read -r TARGET VERSION_FILTER <<< "$TARGET_ARG"
TARGET=${TARGET:-"all"}

echo -e "${BLUE}Building Laravel PaaS Runtime Images (Target: ${TARGET_ARG})...${NC}"

# Versions to build
ALL_VERSIONS=("8.0" "8.1" "8.2" "8.3" "8.4")
VERSIONS=()

if [ -n "$VERSION_FILTER" ]; then
    VERSIONS=("$VERSION_FILTER")
else
    VERSIONS=("${ALL_VERSIONS[@]}")
fi

# Initialize builder if remote BuildKit is running and builder not created yet
if docker ps --format '{{.Names}}' | grep -q "^paas-buildkit$"; then
    if ! docker buildx inspect paas-builder >/dev/null 2>&1; then
        echo -e "${YELLOW}Creating paas-builder buildx remote driver targeting BuildKit...${NC}"
        docker buildx create --name paas-builder --driver remote tcp://127.0.0.1:1234 --use || true
    fi
fi

# Detect if we should use buildx remote builder
BUILD_CMD="docker build"
if docker buildx inspect paas-builder >/dev/null 2>&1; then
    echo -e "${GREEN}[INFO] BuildKit remote builder (paas-builder) detected. Building inside BuildKit and loading to host...${NC}"
    BUILD_CMD="docker buildx build --builder paas-builder --load"
else
    echo -e "${YELLOW}[INFO] BuildKit remote builder (paas-builder) not found. Building locally on host daemon...${NC}"
fi

for VERSION in "${VERSIONS[@]}"; do
    # 1. Build Base Runtime
    if [[ "$TARGET" == "all" || "$TARGET" == "runtime" ]]; then
        TAG_RUNTIME="paas-runtime-php:${VERSION}-alpine"
        
        if [ "$FORCE_REBUILD" = false ] && docker image inspect "$TAG_RUNTIME" >/dev/null 2>&1; then
            echo -e "${GREEN}[SKIP] PHP ${VERSION} runtime already exists. Use --force to rebuild.${NC}"
        else
            echo -e "${YELLOW}Building PHP ${VERSION} runtime... ($TAG_RUNTIME)${NC}"
            $BUILD_CMD \
                --build-arg PHP_VERSION="${VERSION}" \
                -f "${DOCKER_BASE}" \
                -t "${TAG_RUNTIME}" \
                "${PROJECT_ROOT}/docker/runtime"
            echo -e "${GREEN}[SUCCESS] PHP ${VERSION} runtime built successfully.${NC}"
        fi
    fi

    # 2. Build Unified Builder
    if [[ "$TARGET" == "all" || "$TARGET" == "builder" ]]; then
        TAG_BUILDER="paas-builder-base:${VERSION}-alpine"
        
        if [ "$FORCE_REBUILD" = false ] && docker image inspect "$TAG_BUILDER" >/dev/null 2>&1; then
            echo -e "${GREEN}[SKIP] PHP ${VERSION} Unified Builder already exists. Use --force to rebuild.${NC}"
        else
            echo -e "${YELLOW}Building PHP ${VERSION} Unified Builder... ($TAG_BUILDER)${NC}"
            $BUILD_CMD \
                --build-arg PHP_VERSION="${VERSION}" \
                -f "${DOCKER_BUILDER}" \
                -t "${TAG_BUILDER}" \
                "${PROJECT_ROOT}/docker/runtime"
            echo -e "${GREEN}[SUCCESS] PHP ${VERSION} builder built successfully.${NC}"
        fi
    fi
done

echo -e "${BLUE}[INFO] All selected images are ready!${NC}"
