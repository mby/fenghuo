VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/mby/fenghuo/internal/version.Version=$(VERSION)
GOFLAGS := -trimpath
LINUX_ARCHES := amd64 arm64 arm

.PHONY: all web build server agent cross test check lint vuln dev-server dev-web clean

all: build

web:
	pnpm --dir web install --frozen-lockfile
	pnpm --dir web build

build: web server agent

server:
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/fenghuo-server ./cmd/fenghuo-server

agent:
	CGO_ENABLED=0 GOOS=linux go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/fenghuo-agent ./cmd/fenghuo-agent

cross: web
	for arch in $(LINUX_ARCHES); do \
		CGO_ENABLED=0 GOOS=linux GOARCH=$$arch GOARM=7 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/fenghuo-server-linux-$$arch ./cmd/fenghuo-server || exit 1; \
		CGO_ENABLED=0 GOOS=linux GOARCH=$$arch GOARM=7 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/fenghuo-agent-linux-$$arch ./cmd/fenghuo-agent || exit 1; \
	done

test:
	go test ./...
	pnpm --dir web test
	python3 -m unittest discover -s scripts -p 'test_*.py'
	bash deploy/tests/install_test.sh
	bash deploy/tests/configure_probes_test.sh

lint:
	go vet ./...
	GOOS=linux CGO_ENABLED=0 go vet ./...
	pnpm --dir web lint
	pnpm --dir web typecheck

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# Everything CI runs. Must pass before a change is considered done.
check: lint test
	pnpm --dir web build

# Local development: run the server with both listeners on loopback,
# and `make dev-web` in another terminal for hot reload on http://localhost:5173.
dev-server:
	go run ./cmd/fenghuo-server serve -data ./data -log-level debug

dev-web:
	pnpm --dir web dev

clean:
	rm -rf bin dist
	find internal/server/webui/dist -mindepth 1 ! -name .gitkeep -delete
