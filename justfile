build-web:
    go build -o ./bin/BorealValley-web ./src/cmd/web 

test-web:
    go test ./src/cmd/web 

build-ctl:
    go build -o ./bin/BorealValley-ctl ./src/cmd/ctl

test-ctl:
    go test ./src/cmd/ctl 

build-agent:
    go build -o ./bin/BorealValley-agent ./src/cmd/agent

test-agent:
    go test ./src/cmd/agent

build: build-web build-ctl build-agent
test: test-web test-ctl test-agent

test-integration:
    RUN_INTEGRATION=1 go test -count=1 -tags=integration ./src/cmd/web

clean:
	rm bin/*
	touch bin/.keep

dev-docker-up ROOT PORT='4000':
	./tools/deploy/docker-dev-stack.sh up --root '{{ROOT}}' --port '{{PORT}}'

dev-docker-down ROOT PORT='4000':
	./tools/deploy/docker-dev-stack.sh down --root '{{ROOT}}' --port '{{PORT}}'

dev-docker-reset ROOT PORT='4000' DB_PORT='5432' MODE='parity' KEEP_ROOT='0':
	if [ '{{KEEP_ROOT}}' = '1' ]; then \
		./tools/deploy/docker-dev-reset.sh --root '{{ROOT}}' --port '{{PORT}}' --db-port '{{DB_PORT}}' --mode '{{MODE}}' --keep-root; \
	else \
		./tools/deploy/docker-dev-reset.sh --root '{{ROOT}}' --port '{{PORT}}' --db-port '{{DB_PORT}}' --mode '{{MODE}}'; \
	fi
