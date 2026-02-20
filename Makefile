BINARY_NAME := line
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X github.com/re-cinq/assembly-line/internal/cli.Version=$(VERSION)
BUILD_DIR := bin

.PHONY: build test lint fmt clean install

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/line

test:
	go test ./test/acceptance/... -v

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

clean:
	rm -rf $(BUILD_DIR)

install: build
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME) 2>/dev/null || \
	cp $(BUILD_DIR)/$(BINARY_NAME) $(HOME)/go/bin/$(BINARY_NAME)
