#!/usr/bin/env bash
# Build a production BorealValley container image for a single target platform.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  tools/deploy/docker-build.sh [options]

Options:
  --tag IMAGE_TAG       Image tag to produce (default: borealvalley-web:latest)
  --platform PLATFORM   Target platform (default: linux/amd64)
  --dockerfile PATH     Dockerfile path (default: tools/deploy/Dockerfile.prod)
  --build-arg KEY=VALUE Build argument to pass through (repeatable)
  --no-cache            Disable Docker build cache
  -h, --help            Show this help

Examples:
  tools/deploy/docker-build.sh --tag borealvalley-web:dev
  tools/deploy/docker-build.sh --platform linux/amd64 --tag my-registry/bv-web:v1
EOF
}

TAG="borealvalley-web:latest"
PLATFORM="linux/amd64"
NO_CACHE=0
BUILD_ARGS=()

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
DOCKERFILE="$SCRIPT_DIR/Dockerfile.prod"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tag)
      TAG="${2:-}"
      shift 2
      ;;
    --platform)
      PLATFORM="${2:-}"
      shift 2
      ;;
    --dockerfile)
      DOCKERFILE="${2:-}"
      shift 2
      ;;
    --build-arg)
      BUILD_ARGS+=("${2:-}")
      shift 2
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

if [[ ! -f "$DOCKERFILE" ]]; then
  echo "error: dockerfile not found: $DOCKERFILE" >&2
  exit 2
fi

TARGETOS="${PLATFORM%%/*}"
TARGETARCH="${PLATFORM#*/}"

if [[ "$TARGETOS" == "$PLATFORM" || "$TARGETARCH" == "$PLATFORM" || "$TARGETARCH" == *"/"* ]]; then
  echo "error: unsupported platform format: $PLATFORM" >&2
  exit 2
fi

cmd=(docker build --platform "$PLATFORM" -f "$DOCKERFILE" -t "$TAG")
cmd+=(--build-arg "TARGETOS=$TARGETOS" --build-arg "TARGETARCH=$TARGETARCH")
if [[ "${#BUILD_ARGS[@]}" -gt 0 ]]; then
  for build_arg in "${BUILD_ARGS[@]}"; do
    cmd+=(--build-arg "$build_arg")
  done
fi
if [[ "$NO_CACHE" -eq 1 ]]; then
  cmd+=(--no-cache)
fi
cmd+=("$REPO_ROOT")

printf 'running:'
printf ' %q' "${cmd[@]}"
printf '\n'
"${cmd[@]}"
