FROM golang:1.26-alpine AS builder

RUN apk add --no-cache make gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make build

FROM alpine:3.21

RUN apk add --no-cache ca-certificates
COPY --from=builder /src/build/ota /usr/local/bin/ota

ENTRYPOINT ["ota"]
CMD ["server", "--foreground"]
