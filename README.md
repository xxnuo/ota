# OTA - Over The Air Binary Deploy

[中文文档](README.zh-CN.md)

Push binaries to remote machines and run them instantly. Built for rapid edit-compile-test cycles across networked devices.

## How It Works

```
┌─────────────────────────────┐          ┌──────────────────────────┐
│      Dev Machine            │          │     Target Machine       │
│                             │          │                          │
│  ota server (foreground)    │◄════════►│  ota client              │
│    ├─ HTTP  /send           │ WebSocket│    ├─ receives binary    │
│    ├─ HTTP  /disconnect     │          │    ├─ stops old process  │
│    └─ WS   /ws              │          │    ├─ starts new process │
│                             │          │    └─ streams logs back  │
│  ota send ./build/app ──────┼──►       │                          │
│                             │          │                          │
│  stdout:                    │          │                          │
│    [client] received app    │◄─────────│  app stdout/stderr       │
│    [app:err] listening :80  │          │                          │
└─────────────────────────────┘          └──────────────────────────┘
```

## Quick Start

### Install

```bash
git clone https://github.com/xxnuo/ota.git
cd ota
make install    # builds and copies to ~/.local/bin/ota
```

### 1. Start Server (Dev Machine)

```bash
cd your-project
ota server              # auto-selects port, writes .ota file
ota server -p 9867      # or specify a port
```

The server runs in the foreground. All client and app logs appear here.

### 2. Start Client (Target Machine)

```bash
ota client -s ws://dev-machine:9867 -d /opt/app
```

Or use environment variables:

```bash
export OTA_SERVER=ws://dev-machine:9867
ota client -d /opt/app
```

### 3. Send Binary

On the dev machine, in the same directory where `ota server` is running:

```bash
# build your app, then send
go build -o ./build/app .
ota send ./build/app
```

The client will:
1. Stop the currently running app (SIGTERM, then SIGKILL after 500ms)
2. Write the new binary to its working directory
3. Start the new binary
4. Stream all stdout/stderr back to the server

### 4. Disconnect

```bash
ota disconnect
```

The client will stop the running app and exit.

## Commands

| Command | Description |
|---------|-------------|
| `ota server [-p PORT]` | Start server in foreground (0 = auto port) |
| `ota client -s URL [-d DIR]` | Connect to server and wait for binaries |
| `ota send <file> [--args "..."]` | Send binary to the connected client |
| `ota disconnect` | Disconnect client and make it exit |

## Directory-Based Port File

When `ota server` starts, it writes a `.ota` file containing the port number in the current directory. `ota send` and `ota disconnect` read this file automatically.

This means you can run multiple independent servers in different project directories:

```
~/project-a/  →  ota server (port 41523)  →  .ota contains "41523"
~/project-b/  →  ota server (port 38901)  →  .ota contains "38901"
```

Running `ota send ./app` in `~/project-a/` sends to the correct server automatically.

## Send with Arguments

```bash
ota send ./myapp --args "-port 8080 -config prod.yaml"
```

The client will run: `./myapp -port 8080 -config prod.yaml`

## Docker / Compose

### Dockerfile

```dockerfile
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache make
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make build

FROM alpine:3.21
COPY --from=builder /src/build/ota /usr/local/bin/ota
ENTRYPOINT ["ota"]
```

### docker-compose.yml

```yaml
services:
  server:
    build: .
    command: ["server", "-p", "9867"]
    ports:
      - "9867:9867"

  client:
    build: .
    command: ["client", "-s", "ws://server:9867", "-d", "/workspace"]
```

```bash
docker compose up -d
echo "9867" > .ota   # for local ota send to reach the containerized server
ota send ./build/app
```

## Demo

A demo Go HTTP server is included in `demo/`:

```bash
# Terminal 1: start server
cd ota
ota server -p 9867

# Terminal 2: build and send
cd ota/demo
make send         # build v1 and send
make v2           # build v2 and send (hot swap)
make v3           # build v3 and send
```

## Protocol

Communication uses WebSocket with JSON messages:

| Message | Direction | Purpose |
|---------|-----------|---------|
| `binary` | server → client | Binary file transfer (filename + content + args) |
| `log` | client → server | Log line (source + text) |
| `disconnect` | server → client | Tell client to exit |
| `ping/pong` | bidirectional | Keepalive |

## Build

```bash
make build          # build for current platform
make build-linux    # cross-compile for linux amd64/arm64
make install        # build + install to ~/.local/bin
make test           # run tests
make clean          # remove build artifacts
```

## License

See [LICENSE](LICENSE).
