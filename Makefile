BIN ?= bin/kedify

.PHONY: build fmt vet test

build:
	mkdir -p $(dir $(BIN))
	GOCACHE=/tmp/go-build CGO_ENABLED=0 go build -o $(BIN) ./cmd/kedify

fmt:
	gofmt -w $(shell go list -f '{{.Dir}}' ./...)

vet:
	go vet ./...

test:
	GOCACHE=/tmp/go-build go test ./...
