#!/usr/bin/env bash
# Start/stop a Docker Compose development stack for BorealValley.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  tools/deploy/docker-dev-stack.sh [up|down|logs|ps] --root PATH [options]

Commands:
  up                Build and start the stack (default)
  down              Stop and remove the stack
  logs              Follow web service logs
  ps                Show stack status

Options:
  --root PATH       Host root directory to mount into container (required)
  --port PORT       Host port to publish (default: 4000)
  --db-port PORT    Host PostgreSQL port (default: 5432)
  --mode MODE       Development mode: parity or fast (default: parity)
  --no-build        For "up", skip image build
  -h, --help        Show this help

Notes:
  - Container root mount point is fixed to /work.
  - Container web port is fixed to 4000.
  - Host selected --port maps to container port 4000.
  - parity mode runs compiled binaries from a two-stage build.
  - fast mode runs `go run` for faster edit/run loops.
EOF
}

ACTION="up"
ROOT_DIR=""
HOST_PORT="4000"
DB_PORT="5432"
MODE="parity"
NO_BUILD=0

if [[ $# -gt 0 ]]; then
  case "$1" in
    up|down|logs|ps)
      ACTION="$1"
      shift
      ;;
  esac
fi

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
    --no-build)
      NO_BUILD=1
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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.dev.yml"

mkdir -p "$ROOT_DIR"
ROOT_DIR="$(cd "$ROOT_DIR" && pwd)"

if [[ -f "$ROOT_DIR/config.json" ]] && ! grep -Eq '"port"[[:space:]]*:[[:space:]]*4000' "$ROOT_DIR/config.json"; then
  cat >&2 <<EOF
error: $ROOT_DIR/config.json does not use port 4000.
This compose setup expects container port 4000.
Either use a fresh --root path or edit config.json to set "port": 4000.
EOF
  exit 2
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

# If Docker can't reach a daemon and Colima is installed, default to Colima's socket.
if command -v docker >/dev/null 2>&1 && ! docker info >/dev/null 2>&1; then
  if [[ -z "${DOCKER_HOST:-}" ]]; then
    COLIMA_PROFILE="${COLIMA_PROFILE:-default}"
    COLIMA_SOCK="${HOME}/.colima/${COLIMA_PROFILE}/docker.sock"
    if command -v colima >/dev/null 2>&1 && [[ -S "$COLIMA_SOCK" ]]; then
      export DOCKER_HOST="unix://$COLIMA_SOCK"
    fi
  fi
fi

COMPOSE_MODE=""
if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
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

if command -v docker >/dev/null 2>&1 && ! docker info >/dev/null 2>&1; then
  cat >&2 <<'EOF'
error: Docker daemon is not reachable.
If you are using Colima on macOS, run:
  colima start
  docker context use colima
Then retry this script.
EOF
  exit 1
fi

case "$ACTION" in
  up)
    if [[ "$COMPOSE_MODE" == "docker-compose-plugin" ]]; then
      if [[ "$NO_BUILD" -eq 1 ]]; then
        exec docker compose -f "$COMPOSE_FILE" up
      fi
      exec docker compose -f "$COMPOSE_FILE" up --build
    fi
    if [[ "$NO_BUILD" -eq 1 ]]; then
      exec docker-compose -f "$COMPOSE_FILE" up
    fi
    exec docker-compose -f "$COMPOSE_FILE" up --build
    ;;
  down)
    if [[ "$COMPOSE_MODE" == "docker-compose-plugin" ]]; then
      exec docker compose -f "$COMPOSE_FILE" down
    fi
    exec docker-compose -f "$COMPOSE_FILE" down
    ;;
  logs)
    if [[ "$COMPOSE_MODE" == "docker-compose-plugin" ]]; then
      exec docker compose -f "$COMPOSE_FILE" logs -f web
    fi
    exec docker-compose -f "$COMPOSE_FILE" logs -f web
    ;;
  ps)
    if [[ "$COMPOSE_MODE" == "docker-compose-plugin" ]]; then
      exec docker compose -f "$COMPOSE_FILE" ps
    fi
    exec docker-compose -f "$COMPOSE_FILE" ps
    ;;
esac
