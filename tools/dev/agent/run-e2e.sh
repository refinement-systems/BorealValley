#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  tools/dev/agent/run-e2e.sh [options]

Options:
  --pg-admin-dsn DSN   PostgreSQL DSN used to create/drop temporary test databases.
                       Defaults to $BV_E2E_PG_ADMIN_DSN.
  --run REGEX          go test -run regex (default: TestE2EAgentOAuthToRunFlow)
  --count N            go test -count value (default: 1)
  --verbose            Pass -v to go test
  -h, --help           Show this help

Environment:
  BV_E2E_PG_ADMIN_DSN  Default DSN if --pg-admin-dsn is not provided.

Notes:
  - This script sets RUN_E2E=1 automatically.
  - Rodney, local TLS certs, and bv.local DNS resolution are still required.
USAGE
}

PG_ADMIN_DSN="${BV_E2E_PG_ADMIN_DSN:-}"
RUN_REGEX="TestE2EAgentOAuthToRunFlow"
COUNT="1"
VERBOSE=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --pg-admin-dsn)
      PG_ADMIN_DSN="${2:-}"
      shift 2
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

if [[ -z "$PG_ADMIN_DSN" ]]; then
  echo "error: --pg-admin-dsn is required (or set BV_E2E_PG_ADMIN_DSN)" >&2
  exit 2
fi

if [[ ! "$COUNT" =~ ^[0-9]+$ ]] || (( COUNT < 1 )); then
  echo "error: --count must be a positive integer" >&2
  exit 2
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

CMD=(
  go
  test
  ./src/cmd/agent
  -tags=e2e
  -run "$RUN_REGEX"
  -count "$COUNT"
)

if [[ "$VERBOSE" -eq 1 ]]; then
  CMD+=(-v)
fi

(
  cd "$REPO_ROOT"
  export RUN_E2E=1
  export BV_E2E_PG_ADMIN_DSN="$PG_ADMIN_DSN"
  exec "${CMD[@]}"
)
