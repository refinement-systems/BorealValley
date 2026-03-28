# build the web binary
build-web:
    go build -o ./bin/BorealValley-web ./src/cmd/web

# run web package tests
test-web:
    go test ./src/cmd/web

# build the ctl binary
build-ctl:
    go build -o ./bin/BorealValley-ctl ./src/cmd/ctl

# run ctl package tests
test-ctl:
    go test ./src/cmd/ctl

# build the agent binary
build-agent:
    go build -o ./bin/BorealValley-agent ./src/cmd/agent

# run agent package tests
test-agent:
    go test ./src/cmd/agent

# build all binaries
build: build-web build-ctl build-agent

# run all tests
test:
    go test ./...

# run integration tests (requires running server)
test-integration:
    RUN_INTEGRATION=1 go test -count=1 -tags=integration ./src/cmd/web

# remove built binaries
clean:
	rm bin/*
	touch bin/.keep

# start the dev docker stack
dev-docker-up ROOT PORT='4000':
	./tools/deploy/docker-dev-stack.sh up --root '{{ROOT}}' --port '{{PORT}}'

# stop the dev docker stack
dev-docker-down ROOT PORT='4000':
	./tools/deploy/docker-dev-stack.sh down --root '{{ROOT}}' --port '{{PORT}}'

# reset the dev docker stack
dev-docker-reset ROOT PORT='4000' DB_PORT='5432' MODE='parity':
	./tools/deploy/docker-dev-reset.sh --root '{{ROOT}}' --port '{{PORT}}' --db-port '{{DB_PORT}}' --mode '{{MODE}}'
