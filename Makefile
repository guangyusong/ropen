.PHONY: build test install

build:
	go build ./...

test:
	go test ./...

install:
	go install .
