BINARY=ota
BUILD_DIR=build
VERSION ?= dev
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: all build test clean install lint fmt run-server run-client \
       docker-test demo-build demo-up demo-down demo-restart demo-logs demo-ps tidy

all: build

build:
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/ota

build-linux:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./cmd/ota
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-arm64 ./cmd/ota

build-all: build-linux
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-amd64 ./cmd/ota
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-arm64 ./cmd/ota

test:
	go test -v -race -count=1 ./...

test-unit:
	go test -v -race -count=1 ./internal/...

test-integration:
	go test -v -race -count=1 ./tests/...

test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

install: build
	cp $(BUILD_DIR)/$(BINARY) $(GOPATH)/bin/ 2>/dev/null || cp $(BUILD_DIR)/$(BINARY) /usr/local/bin/

fmt:
	gofmt -s -w .

lint:
	golangci-lint run ./...

run-server:
	go run ./cmd/ota server --port 9867 --dir .

run-client:
	go run ./cmd/ota client --server ws://localhost:9867 --dir /tmp/ota-client-test --cmd "echo synced"

docker-test:
	docker compose -f docker-compose.test.yml up --build --abort-on-container-exit
	docker compose -f docker-compose.test.yml down -v

demo-build:
	docker compose -f docker-compose.demo.yml build

demo-up:
	docker compose -f docker-compose.demo.yml up --build -d

demo-down:
	docker compose -f docker-compose.demo.yml down -v

demo-restart:
	docker compose -f docker-compose.demo.yml down -v
	docker compose -f docker-compose.demo.yml up --build -d

demo-logs:
	docker compose -f docker-compose.demo.yml logs -f

demo-ps:
	docker compose -f docker-compose.demo.yml ps

tidy:
	go mod tidy
