#!/bin/bash
set -e

IMAGE_NAME=${1:-my-project}
DOCKER_PLATFORM=${2:-linux/amd64}

docker build --platform "$DOCKER_PLATFORM" -f benzhi.Dockerfile -t "$IMAGE_NAME" .

echo ""
echo "✅ Docker image '$IMAGE_NAME' built successfully!"
echo ""
echo "📋 Next steps (for testing):"
echo "  • Run smoke test: docker run --rm $IMAGE_NAME:latest --smoke-test"
echo "  • Start server:   docker run --rm -p 8080:8080 $IMAGE_NAME:latest --addr :8080"
