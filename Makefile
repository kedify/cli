BIN ?= bin/kedify
GOLANGCI_LINT_VERSION ?= v1.64.8
GOLANGCI_LINT := $(CURDIR)/bin/golangci-lint

.PHONY: build fmt vet test golangci-lint golangci-lint-bin

build:
	mkdir -p $(dir $(BIN))
	GOCACHE=/tmp/go-build CGO_ENABLED=0 go build -o $(BIN) ./cmd/kedify

fmt:
	gofmt -w $(shell go list -f '{{.Dir}}' ./...)

vet:
	go vet ./...

test:
	GOCACHE=/tmp/go-build go test ./...

golangci-lint-bin:
	@mkdir -p $(dir $(GOLANGCI_LINT))
	@if [ ! -x "$(GOLANGCI_LINT)" ] || ! "$(GOLANGCI_LINT)" version | grep -q "$(GOLANGCI_LINT_VERSION)"; then \
		echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)"; \
		GOBIN="$(CURDIR)/bin" go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi

golangci-lint: golangci-lint-bin
	GOCACHE=/tmp/go-build GOLANGCI_LINT_CACHE=/tmp/golangci-lint "$(GOLANGCI_LINT)" run
