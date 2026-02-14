#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  tools/deploy/docker-dev-agent-e2e.sh --root PATH [options]

Options:
  --root PATH       Host root directory for the Docker dev stack (required)
  --port PORT       Host port for the Docker dev web service (default: 4000)
  --db-port PORT    Host PostgreSQL port for the Docker dev DB (default: 5432)
  --mode MODE       Docker dev mode: parity or fast (default: parity)
  --no-build        Pass through to docker-dev-stack.sh up
  --keep-stack      Leave the Docker dev stack running after the test
  --run REGEX       go test -run regex (default: TestE2EAgentOAuthToRunFlow)
  --count N         go test -count value (default: 1)
  --verbose         Pass -v to go test
  -h, --help        Show this help

Environment:
  BV_DEV_DB_USER        Docker dev DB user (default: app)
  BV_DEV_DB_PASSWORD    Docker dev DB password (default: app_pw)
  BV_DEV_DB_NAME        Docker dev DB name (default: app_db)

Notes:
  - This wrapper starts the existing Docker dev stack, reuses its PostgreSQL
    service for the e2e test, then stops the stack unless --keep-stack is set.
  - The e2e test still launches its own BorealValley web/ctl/agent processes.
USAGE
}

ROOT_DIR=""
HOST_PORT="4000"
DB_PORT="5432"
MODE="parity"
NO_BUILD=0
KEEP_STACK=0
RUN_REGEX="TestE2EAgentOAuthToRunFlow"
COUNT="1"
VERBOSE=0

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
    --keep-stack)
      KEEP_STACK=1
      shift
      ;;
    --run)
      RUN_REGEX="${2:-}"
      shift 2
      ;;
    --count)
      COUNT="${2:-}"
      shift 2
      ;;
    --verbose)
      VERBOSE=1
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

if [[ ! "$COUNT" =~ ^[0-9]+$ ]] || (( COUNT < 1 )); then
  echo "error: --count must be a positive integer" >&2
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
STACK_SCRIPT="$SCRIPT_DIR/docker-dev-stack.sh"
RUN_E2E_SCRIPT="$REPO_ROOT/tools/dev/agent/run-e2e.sh"

DB_USER="${BV_DEV_DB_USER:-app}"
DB_PASSWORD="${BV_DEV_DB_PASSWORD:-app_pw}"
DB_NAME="${BV_DEV_DB_NAME:-app_db}"
PG_ADMIN_DSN="postgres://${DB_USER}:${DB_PASSWORD}@127.0.0.1:${DB_PORT}/${DB_NAME}?sslmode=disable"

STACK_UP=0
STACK_PID=""
STACK_LOG=""
cleanup() {
  if [[ "$STACK_UP" -eq 1 && "$KEEP_STACK" -eq 0 ]]; then
    "$STACK_SCRIPT" down --root "$ROOT_DIR" >/dev/null
    if [[ -n "$STACK_PID" ]] && kill -0 "$STACK_PID" >/dev/null 2>&1; then
      kill "$STACK_PID" >/dev/null 2>&1 || true
      wait "$STACK_PID" 2>/dev/null || true
    fi
  fi
  if [[ -n "$STACK_LOG" && -f "$STACK_LOG" ]]; then
    rm -f "$STACK_LOG"
  fi
}
trap cleanup EXIT

STACK_CMD=("$STACK_SCRIPT" up --root "$ROOT_DIR" --port "$HOST_PORT" --db-port "$DB_PORT" --mode "$MODE")
if [[ "$NO_BUILD" -eq 1 ]]; then
  STACK_CMD+=(--no-build)
fi
STACK_LOG="$(mktemp -t docker-dev-agent-e2e.XXXXXX.log)"
nohup "${STACK_CMD[@]}" >"$STACK_LOG" 2>&1 &
STACK_PID="$!"

deadline=$((SECONDS + 120))
until bash -c "</dev/tcp/127.0.0.1/${DB_PORT}" >/dev/null 2>&1; do
  if (( SECONDS >= deadline )); then
    echo "error: timed out waiting for Docker dev PostgreSQL on port ${DB_PORT}" >&2
    if [[ -f "$STACK_LOG" ]]; then
      echo "--- docker-dev-stack log ---" >&2
      cat "$STACK_LOG" >&2
    fi
    exit 1
  fi
  if ! kill -0 "$STACK_PID" >/dev/null 2>&1; then
    echo "error: docker-dev-stack.sh exited before PostgreSQL became ready" >&2
    if [[ -f "$STACK_LOG" ]]; then
      echo "--- docker-dev-stack log ---" >&2
      cat "$STACK_LOG" >&2
    fi
    exit 1
  fi
  sleep 2
done
STACK_UP=1

TEST_CMD=("$RUN_E2E_SCRIPT" --pg-admin-dsn "$PG_ADMIN_DSN" --run "$RUN_REGEX" --count "$COUNT")
if [[ "$VERBOSE" -eq 1 ]]; then
  TEST_CMD+=(--verbose)
fi
"${TEST_CMD[@]}"

if [[ "$KEEP_STACK" -eq 1 ]]; then
  echo "Docker dev stack left running at root: $ROOT_DIR" >&2
fi
