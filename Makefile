VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build test install doctor clean

build:
	go build -ldflags "$(LDFLAGS)" ./...

test:
	go test ./...

install:
	go install -ldflags "$(LDFLAGS)" .

doctor:
	go run . doctor

clean:
	rm -rf dist ropen
