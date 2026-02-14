#!/usr/bin/env bash
# Downloads the latest pinned versions of htmx and htmx-ext-sse into assets/js/.
set -euo pipefail

HTMX_VERSION="2.0.4"
SSE_VERSION="2.2.2"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="$SCRIPT_DIR/../src/internal/assets/js"

mkdir -p "$OUT_DIR"

echo "Downloading htmx $HTMX_VERSION..."
curl -fsSL -o "$OUT_DIR/htmx.min.js" \
  "https://unpkg.com/htmx.org@${HTMX_VERSION}/dist/htmx.min.js"

echo "Downloading htmx-ext-sse $SSE_VERSION..."
curl -fsSL -o "$OUT_DIR/sse.js" \
  "https://unpkg.com/htmx-ext-sse@${SSE_VERSION}/sse.js"

echo "Done. Files written to $OUT_DIR/"
