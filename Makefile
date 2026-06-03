GO ?= go
BINARY_NAME = symmemory

.PHONY: all
all: build test

.PHONY: build
build:
	$(GO) build -o $(BINARY_NAME) main.go

.PHONY: test
test:
	$(GO) test -v ./...

.PHONY: lint
lint:
	$(GO) fmt ./...
	$(GO) vet ./...

.PHONY: clean
clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/
