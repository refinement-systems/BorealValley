# Local Docker Dev Stack

This guide covers the standard BorealValley local Docker development setup and, in particular, how to reopen an environment that was already set up earlier.

## 1. Resume the Existing Local Environment

If the standard local dev stack was already created, the normal command to bring it back is:

```bash
cd /Users/mjm/repo/BorealValley
just dev-docker-up ~/repo/bvroot
```

That command is the canonical "start or resume" path for the shared local setup:

- it mounts `~/repo/bvroot` into the container as `/work`,
- it starts the web server on host port `4000`,
- it starts PostgreSQL on host port `5432`,
- it reuses the existing Docker PostgreSQL volume when the stack was previously stopped with `down`,
- it keeps the BorealValley root files in `~/repo/bvroot`.

After startup, open:

```text
https://bv.local:4000/web/login
```

In dev mode the web server automatically uses `cert/bv.local+3.pem` and `cert/bv.local+3-key.pem` when those files exist in the repository, so the standard local URL is HTTPS.

## 2. What State Gets Reused

When you restart with `just dev-docker-up ~/repo/bvroot`, the following state is expected to come back:

- BorealValley root config in `~/repo/bvroot/config.json`
- local discovered repositories under `~/repo/bvroot/repo/`
- PostgreSQL data from the Docker Compose `pgdata` volume
- users, tickets, tracker assignments, OAuth apps, and prior agent-created comments stored in PostgreSQL

Agent-side OAuth state is separate from the server state. If you initialized the local agent before, its default state file is:

```text
~/.local/state/BorealValley/agent/state.json
```

## 3. Stop, Inspect, and Reset

Stop the stack without deleting state:

```bash
cd /Users/mjm/repo/BorealValley
just dev-docker-down ~/repo/bvroot
```

Inspect the running stack:

```bash
./tools/deploy/docker-dev-stack.sh ps --root ~/repo/bvroot
```

Follow web logs:

```bash
./tools/deploy/docker-dev-stack.sh logs --root ~/repo/bvroot
```

Reset the stack to a fresh PostgreSQL database:

```bash
cd /Users/mjm/repo/BorealValley
just dev-docker-reset ~/repo/bvroot
```

`just dev-docker-reset ~/repo/bvroot` is destructive for database state. It preserves the root directory on disk, but it removes the Docker PostgreSQL volume and starts with an empty database. Do not use it when you want to keep an existing test run.

## 4. Run the Web Server Outside Docker Against the Same Data

If you want the same BorealValley root and PostgreSQL state but want the web process to run directly on the host instead of in Docker, use the same root path and DSN:

```bash
cd /Users/mjm/repo/BorealValley
export BV_PG_DSN='postgres://app:app_pw@127.0.0.1:5432/app_db?sslmode=disable'
go run ./src/cmd/web serve --root ~/repo/bvroot --env dev
```

That command uses the current branch's UI and server code while pointing at the same persisted development data.

## 5. If `~/repo/bvroot` Does Not Exist

This guide assumes the local Docker environment was already initialized in the standard place. If `~/repo/bvroot/config.json` is missing, you are not resuming the shared local setup and should create a fresh root instead of expecting old state to appear.
