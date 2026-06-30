# Pocketknife runtime — single generic, schema-driven backend.
#
# Go is expected on PATH. If you installed it under ~/.local/go (no Homebrew),
# run: export PATH="$$HOME/.local/go/bin:$$PATH"

GO ?= go
ADDR ?= :8080
APPS ?= apps

SHELL_DIR ?= shell

.PHONY: all build shell-build run test vet fmt clean tidy

all: build

shell-build:
	cd $(SHELL_DIR) && npm install && npm run build

build: shell-build
	$(GO) build -o bin/pocketknife ./cmd/pocketknife

run:
	$(GO) run ./cmd/pocketknife -addr $(ADDR) -apps $(APPS)

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

clean:
	rm -rf bin
	find $(APPS) -name 'data.db' -delete
