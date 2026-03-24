GO ?= go

.PHONY: test test-short run fmt

test:
	$(GO) test ./...

test-short:
	$(GO) test -short ./...

run:
	$(GO) run ./cmd/server

fmt:
	$(GO) fmt ./...
