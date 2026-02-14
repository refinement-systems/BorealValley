#!/usr/bin/env sh

(set -x; brew install colima)
(set -x; brew services start colima)
(set -x; brew install docker)
(set -x; brew install docker-compose)

