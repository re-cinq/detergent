BINARY_NAME := line
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X github.com/re-cinq/assembly-line/internal/cli.Version=$(VERSION)
BUILD_DIR := bin

.PHONY: build test lint fmt clean install

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/line
	@if [ "$$(uname)" = "Darwin" ]; then codesign --force -s - $(BUILD_DIR)/$(BINARY_NAME); fi

test:
	go test ./test/acceptance/... -v

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

clean:
	rm -rf $(BUILD_DIR)

install: build
	@DEST=$$([ -d "$(GOPATH)/bin" ] && echo "$(GOPATH)/bin" || echo "$(HOME)/go/bin"); \
	rm -f "$$DEST/$(BINARY_NAME)"; \
	cp $(BUILD_DIR)/$(BINARY_NAME) "$$DEST/$(BINARY_NAME)"
