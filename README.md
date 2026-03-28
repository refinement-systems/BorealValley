Yet another ticket tracker for LLM agents.

This is not even close to operational, and published only for curiosity purposes.

## Quick Start (Docker)

The easiest way to run BorealValley locally is with the Docker dev stack:

```sh
just dev-docker-up ~/repo/bvroot
```

Then open `https://bv.local:4000/web/login`. The stack includes PostgreSQL and
mounts your root directory at `/work` inside the container.

To create your first user against the running stack:

```sh
export BV_PG_DSN='postgres://app:app_pw@127.0.0.1:5432/app_db?sslmode=disable'
go run ./src/cmd/ctl adduser --root ~/repo/bvroot --admin <username> <password>
```

Password must be at least 12 characters.

## Manual Setup

**Prerequisites:** Go 1.21+, PostgreSQL

```sh
# 1. Build the binaries
just build

# 2. Initialize the root directory (creates config.json and repo/)
bin/BorealValley-ctl init-root --root ~/bvroot

# 3. Edit ~/bvroot/config.json to set hostname and port
#    Default: {"hostname": "bv.local", "port": 4000}

# 4. Create a user
export BV_PG_DSN='postgres://user:pass@localhost:5432/dbname?sslmode=disable'
bin/BorealValley-ctl adduser --root ~/bvroot --admin <username> <password>

# 5. Start the web server
bin/BorealValley-web serve --root ~/bvroot --env dev
```

## Key Flags and Environment Variables

| Binary | Flag | Description |
|--------|------|-------------|
| `web serve` | `--root PATH` | Root directory (default: `~/.local/share/BorealValley`) |
| `web serve` | `--env dev\|prod` | Environment mode (default: `dev`) |
| `web serve` | `--cert FILE --key FILE` | TLS certificate and key (enables HTTPS) |
| `ctl *` | `--root PATH` | Root directory |
| `ctl adduser` | `--admin` | Grant admin privileges |

| Variable | Description |
|----------|-------------|
| `BV_PG_DSN` | PostgreSQL connection string (required) |
| `XDG_DATA_HOME` | Overrides default root parent directory |

## Further Documentation

- `doc/usage/` — Operator guides including Docker dev stack details
- `doc/spec/` — Architecture and behavior specifications
- `CLAUDE.md` — Build commands and architecture notes for Claude Code
- `AGENTS.md` — Guidelines for LLM agents working in this repo
