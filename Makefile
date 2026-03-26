BINARY=ota
BUILD_DIR=build
VERSION ?= dev
LDFLAGS=-ldflags "-s -w"

.PHONY: all build install clean test tidy

all: build

build:
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/ota

build-linux:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./cmd/ota
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-arm64 ./cmd/ota

test:
	go test -v -race -count=1 ./...

install: build
	@mkdir -p ~/.local/bin
	cp $(BUILD_DIR)/$(BINARY) ~/.local/bin/$(BINARY)
	@echo "installed to ~/.local/bin/$(BINARY)"

clean:
	rm -rf $(BUILD_DIR)

tidy:
	go mod tidy
