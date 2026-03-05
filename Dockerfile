FROM golang:1.25-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    git ca-certificates gcc g++ libc6-dev && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /prysm
COPY go.mod go.sum ./
COPY third_party/ third_party/
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /usr/local/bin/beacon-chain ./cmd/beacon-chain
RUN CGO_ENABLED=1 go build -o /usr/local/bin/validator ./cmd/validator

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/local/bin/beacon-chain /beacon-chain
COPY --from=builder /usr/local/bin/validator /validator

ENTRYPOINT ["/beacon-chain"]
