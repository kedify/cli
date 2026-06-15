BIN ?= bin/kedify

.PHONY: build

build:
	mkdir -p $(dir $(BIN))
	GOCACHE=/tmp/go-build CGO_ENABLED=0 go build -o $(BIN) ./cmd/kedify
