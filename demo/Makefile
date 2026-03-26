VERSION ?= 1
BINARY = demo

.PHONY: build send clean

build:
	CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BINARY) .

send: build
	../build/ota send ./$(BINARY)

v2:
	$(MAKE) send VERSION=2

v3:
	$(MAKE) send VERSION=3

clean:
	rm -f $(BINARY)
