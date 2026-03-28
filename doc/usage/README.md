# Usage Guides

## Scope

This directory contains operator-focused guides for running BorealValley features.

## Quick Start

To reopen the standard local development environment with the existing Docker-backed state, run:

```bash
cd /Users/mjm/repo/BorealValley
just dev-docker-up ~/repo/bvroot
```

Use `just dev-docker-down ~/repo/bvroot` to stop it without deleting state.
Do not use `just dev-docker-reset ~/repo/bvroot` unless you want a fresh PostgreSQL database.

## Guides

- `docker-dev-stack.md`: start, resume, inspect, or reset the standard local Docker dev stack rooted at `~/repo/bvroot`.
- `agent-running-workflow.md`: end-to-end setup and manual run workflow for `BorealValley-agent` against a live server.
- `tla-model-checking.md`: running the TLA+ model checker (TLC) against the agent formal specification.
