GO ?= go
GORELEASER ?= goreleaser
BINARY = bin/apix

.PHONY: build test compat vet format check release-check release-snapshot clean

build:
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -o $(BINARY) ./cmd/apix

test: build
	$(GO) test ./...

compat: build
	$(GO) test ./cmd/apix -run '^TestCLI'

vet:
	$(GO) vet ./...

format:
	$(GO) fmt ./...

check: format vet test

release-check:
	$(GORELEASER) check

release-snapshot:
	$(GORELEASER) release --snapshot --clean

clean:
	rm -rf bin dist
