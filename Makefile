.PHONY: build run run-headless stop-headless test vet

GOCACHE ?= /tmp/linx-go-cache
GOFLAGS ?= -buildvcs=false

build:
	mkdir -p bin
	GOCACHE=$(GOCACHE) go build $(GOFLAGS) -o bin/linx ./cmd/linx

run:
	GOCACHE=$(GOCACHE) go run $(GOFLAGS) ./cmd/linx

run-headless: build
	./scripts/run-headless.sh

stop-headless:
	./scripts/launch-chrome.sh --stop

test:
	GOCACHE=$(GOCACHE) go test $(GOFLAGS) ./...

vet:
	GOCACHE=$(GOCACHE) go vet $(GOFLAGS) ./...
