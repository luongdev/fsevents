#!/bin/bash

# Build script for FreeSWITCH ESL Sidecar Docker image

set -e

# Variables
IMAGE_NAME="luongdev/fsevents"
VERSION=${1:-"latest"}
REGISTRY=${DOCKER_REGISTRY:-""}
BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}Building FreeSWITCH ESL Sidecar Docker image...${NC}"
echo "Image: ${IMAGE_NAME}:${VERSION}"
echo "Build Date: ${BUILD_DATE}"
echo "Git Commit: ${GIT_COMMIT}"
echo

# Build the image
echo -e "${YELLOW}Building Docker image...${NC}"
docker buildx build --platform=linux/amd64 --push \
    --build-arg BUILD_DATE="${BUILD_DATE}" \
    --build-arg GIT_COMMIT="${GIT_COMMIT}" \
    --build-arg VERSION="${VERSION}" \
    -t "${IMAGE_NAME}:${VERSION}" \
    -t "${IMAGE_NAME}:latest" \
    .

echo -e "${GREEN}✅ Docker image built successfully!${NC}"

# Show image info
echo -e "${BLUE}Image information:${NC}"
docker images "${IMAGE_NAME}" --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}\t{{.CreatedAt}}"

# Optional: Tag for registry
if [ -n "${REGISTRY}" ]; then
    echo -e "${YELLOW}Tagging for registry: ${REGISTRY}${NC}"
    docker tag "${IMAGE_NAME}:${VERSION}" "${REGISTRY}/${IMAGE_NAME}:${VERSION}"
    docker tag "${IMAGE_NAME}:latest" "${REGISTRY}/${IMAGE_NAME}:latest"
    echo -e "${GREEN}✅ Images tagged for registry${NC}"
fi

echo
echo -e "${GREEN}Build completed successfully!${NC}"
echo
echo "To run the container:"
echo "  docker run -d --name fsevents-sidecar -p 9090:9090 -v \$(pwd)/configs:/app/configs:ro ${IMAGE_NAME}:${VERSION}"
echo
echo "Or use docker-compose:"
echo "  docker-compose up -d"

# Optional: Push to registry
if [ -n "${REGISTRY}" ] && [ "${PUSH_IMAGE:-false}" = "true" ]; then
    echo -e "${YELLOW}Pushing to registry...${NC}"
    docker push "${REGISTRY}/${IMAGE_NAME}:${VERSION}"
    docker push "${REGISTRY}/${IMAGE_NAME}:latest"
    echo -e "${GREEN}✅ Images pushed to registry${NC}"
fi 