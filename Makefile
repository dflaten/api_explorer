GO ?= go
GORELEASER ?= goreleaser
STATICCHECK ?= $(shell $(GO) env GOPATH)/bin/staticcheck
BINARY = bin/apix

.PHONY: build test compat vet staticcheck format check release-check release-snapshot clean

build:
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -o $(BINARY) ./cmd/apix

test: build
	$(GO) test ./...

compat: build
	$(GO) test ./cmd/apix -run '^TestCLI'

vet:
	$(GO) vet ./...

staticcheck:
	$(STATICCHECK) ./...

format:
	$(GO) fmt ./...

check: format vet staticcheck test

release-check:
	$(GORELEASER) check

release-snapshot:
	$(GORELEASER) release --snapshot --clean

clean:
	rm -rf bin dist
