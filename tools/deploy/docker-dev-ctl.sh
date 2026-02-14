#!/usr/bin/env bash
# Run BorealValley ctl inside the Docker Compose dev web container.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  tools/deploy/docker-dev-ctl.sh ARGS...

Examples:
  tools/deploy/docker-dev-ctl.sh adduser --root /work alice supersecretpass
  tools/deploy/docker-dev-ctl.sh resync --root /work

This is equivalent to:
  docker compose -f tools/deploy/docker-compose.dev.yml exec web \
    /app/BorealValley-ctl ARGS...
EOF
}

if [[ $# -eq 0 ]]; then
  usage >&2
  exit 2
fi

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.dev.yml"

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

if [[ "$COMPOSE_MODE" == "docker-compose-plugin" ]]; then
  exec docker compose -f "$COMPOSE_FILE" exec web /app/BorealValley-ctl "$@"
fi

exec docker-compose -f "$COMPOSE_FILE" exec web /app/BorealValley-ctl "$@"
