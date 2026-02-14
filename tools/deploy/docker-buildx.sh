#!/usr/bin/env bash
# Build BorealValley container images with docker buildx for cross-platform targets.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  tools/deploy/docker-buildx.sh [options]

Options:
  --tag IMAGE_TAG         Image tag to produce (default: borealvalley-web:latest)
  --platforms LIST        Comma-separated platforms (default: linux/amd64,linux/arm64)
  --dockerfile PATH       Dockerfile path (default: tools/deploy/Dockerfile.prod)
  --push                  Push image/manifest to registry
  --load                  Load single-platform image into local Docker daemon
  --no-cache              Disable build cache
  -h, --help              Show this help

Rules:
  - Use --push for true multi-platform output.
  - --load supports only a single platform.

Examples:
  tools/deploy/docker-buildx.sh --platforms linux/amd64 --load --tag borealvalley-web:amd64
  tools/deploy/docker-buildx.sh --push --tag ghcr.io/example/borealvalley-web:latest
EOF
}

TAG="borealvalley-web:latest"
PLATFORMS="linux/amd64,linux/arm64"
PUSH=0
LOAD=0
NO_CACHE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
DOCKERFILE="$SCRIPT_DIR/Dockerfile.prod"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tag)
      TAG="${2:-}"
      shift 2
      ;;
    --platforms)
      PLATFORMS="${2:-}"
      shift 2
      ;;
    --dockerfile)
      DOCKERFILE="${2:-}"
      shift 2
      ;;
    --push)
      PUSH=1
      shift
      ;;
    --load)
      LOAD=1
      shift
      ;;
    --no-cache)
      NO_CACHE=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if ! command -v docker >/dev/null 2>&1; then
  echo "error: docker not found in PATH" >&2
  exit 1
fi

if ! docker buildx version >/dev/null 2>&1; then
  echo "error: docker buildx is not available" >&2
  exit 1
fi

if [[ ! -f "$DOCKERFILE" ]]; then
  echo "error: dockerfile not found: $DOCKERFILE" >&2
  exit 2
fi

if [[ "$PUSH" -eq 1 && "$LOAD" -eq 1 ]]; then
  echo "error: --push and --load are mutually exclusive" >&2
  exit 2
fi

if [[ "$PUSH" -eq 0 && "$LOAD" -eq 0 ]]; then
  if [[ "$PLATFORMS" == *,* ]]; then
    echo "error: multiple platforms selected; use --push for multi-platform output" >&2
    exit 2
  fi
  LOAD=1
fi

if [[ "$LOAD" -eq 1 && "$PLATFORMS" == *,* ]]; then
  echo "error: --load only supports a single platform; use --push for multi-platform output" >&2
  exit 2
fi

cmd=(docker buildx build --platform "$PLATFORMS" -f "$DOCKERFILE" -t "$TAG")
if [[ "$NO_CACHE" -eq 1 ]]; then
  cmd+=(--no-cache)
fi
if [[ "$PUSH" -eq 1 ]]; then
  cmd+=(--push)
else
  cmd+=(--load)
fi
cmd+=("$REPO_ROOT")

printf 'running:'
printf ' %q' "${cmd[@]}"
printf '\n'
"${cmd[@]}"
