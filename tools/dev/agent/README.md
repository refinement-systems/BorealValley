# Agent Helper Scripts

Helper scripts for live/manual `BorealValley-agent` operation.

## Scripts

- `create-oauth-app.sh`: creates an OAuth app with required agent scopes.
- `init-agent.sh`: runs one-time `agent init` with common defaults.
- `run-once.sh`: runs one `agent run` invocation against one ticket.
- `run-e2e.sh`: runs the opt-in agent OAuth-to-run e2e test against a provided PostgreSQL admin DSN.

For a Docker-backed local PostgreSQL workflow, use `tools/deploy/docker-dev-agent-e2e.sh`.

Run each script with `--help` for arguments.
