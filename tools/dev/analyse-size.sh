#!/usr/bin/env sh

BINARY="${1:-bin/BorealValley-web}"

exec gsa --format=text ${BINARY} 2> /dev/null
