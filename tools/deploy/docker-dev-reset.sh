#!/usr/bin/env bash
# Fully reset the Docker Compose development deployment and start fresh.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  tools/deploy/docker-dev-reset.sh --root PATH [options]

Options:
  --root PATH       Host root directory mounted into container (required)
  --port PORT       Host web port (default: 4000)
  --db-port PORT    Host PostgreSQL port (default: 5432)
  --mode MODE       Development mode: parity or fast (default: parity)
  --keep-root       Deprecated no-op (root is always preserved)
  -h, --help        Show this help

Behavior:
  1. Stops the dev stack.
  2. Removes development containers and database volume.
  3. Preserves --root contents.
  4. Starts the stack again.
EOF
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: required command not found: $1" >&2
    exit 1
  fi
}

ROOT_DIR=""
HOST_PORT="4000"
DB_PORT="5432"
MODE="parity"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --root)
      ROOT_DIR="${2:-}"
      shift 2
      ;;
    --port)
      HOST_PORT="${2:-}"
      shift 2
      ;;
    --db-port)
      DB_PORT="${2:-}"
      shift 2
      ;;
    --mode)
      MODE="${2:-}"
      shift 2
      ;;
    --keep-root)
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

if [[ -z "$ROOT_DIR" ]]; then
  echo "error: --root is required" >&2
  usage >&2
  exit 2
fi

if [[ ! "$HOST_PORT" =~ ^[0-9]+$ ]] || (( HOST_PORT < 1 || HOST_PORT > 65535 )); then
  echo "error: invalid --port '$HOST_PORT' (must be 1..65535)" >&2
  exit 2
fi

if [[ ! "$DB_PORT" =~ ^[0-9]+$ ]] || (( DB_PORT < 1 || DB_PORT > 65535 )); then
  echo "error: invalid --db-port '$DB_PORT' (must be 1..65535)" >&2
  exit 2
fi

case "$MODE" in
  parity|fast)
    ;;
  *)
    echo "error: invalid --mode '$MODE' (must be parity or fast)" >&2
    exit 2
    ;;
esac

require_cmd docker

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
STACK_SCRIPT="$SCRIPT_DIR/docker-dev-stack.sh"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.dev.yml"

mkdir -p "$ROOT_DIR"
ROOT_DIR="$(cd "$ROOT_DIR" && pwd)"

COMPOSE_MODE=""
if docker compose version >/dev/null 2>&1; then
  COMPOSE_MODE="docker-compose-plugin"
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE_MODE="docker-compose-v1"
else
  cat >&2 <<'EOF'
error: Docker Compose is not available.
Install either:
  - Docker Compose plugin ("docker compose"), or
  - docker-compose binary.
EOF
  exit 1
fi

# If Docker can't reach a daemon and Colima is installed, default to Colima's socket.
if ! docker info >/dev/null 2>&1; then
  if [[ -z "${DOCKER_HOST:-}" ]]; then
    COLIMA_PROFILE="${COLIMA_PROFILE:-default}"
    COLIMA_SOCK="${HOME}/.colima/${COLIMA_PROFILE}/docker.sock"
    if command -v colima >/dev/null 2>&1 && [[ -S "$COLIMA_SOCK" ]]; then
      export DOCKER_HOST="unix://$COLIMA_SOCK"
    fi
  fi
fi

if ! docker info >/dev/null 2>&1; then
  cat >&2 <<'EOF'
error: Docker daemon is not reachable.
If you are using Colima on macOS, run:
  colima start
  docker context use colima
Then retry this script.
EOF
  exit 1
fi

export BV_DEV_REPO_ROOT="$REPO_ROOT"
export BV_DEV_ROOT_HOST="$ROOT_DIR"
export BV_DEV_HOST_PORT="$HOST_PORT"
export BV_DEV_DB_PORT="$DB_PORT"
export BV_DEV_CONTAINER_ROOT="/work"
export BV_DEV_CONTAINER_PORT="4000"
if [[ "$MODE" == "fast" ]]; then
  export BV_DEV_MODE="fast"
  export BV_DEV_DOCKER_TARGET="dev-fast"
else
  export BV_DEV_MODE="parity"
  export BV_DEV_DOCKER_TARGET="dev-runtime"
fi

echo "Stopping dev stack..."
"$STACK_SCRIPT" down --root "$ROOT_DIR" --port "$HOST_PORT" --db-port "$DB_PORT" --mode "$MODE" || true

echo "Removing development containers and database volume..."
if [[ "$COMPOSE_MODE" == "docker-compose-plugin" ]]; then
  docker compose -f "$COMPOSE_FILE" down -v --remove-orphans || true
else
  docker-compose -f "$COMPOSE_FILE" down -v --remove-orphans || true
fi

echo "Keeping root path contents: $ROOT_DIR"
echo "note: if startup fails due to stale config, you may need to replace $ROOT_DIR/config.json."

echo "Starting fresh dev stack..."
exec "$STACK_SCRIPT" up --root "$ROOT_DIR" --port "$HOST_PORT" --db-port "$DB_PORT" --mode "$MODE"
