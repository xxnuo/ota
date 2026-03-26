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
