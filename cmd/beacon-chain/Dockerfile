FROM golang:1.25.1-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
COPY third_party/go-bip39 ./third_party/go-bip39
RUN go mod download
COPY . .
ARG CGO_CFLAGS="-O2 -D__BLST_PORTABLE__"
RUN go build -o /app ./cmd/beacon-chain

FROM gcr.io/distroless/cc-debian12:nonroot
COPY --from=builder /app /usr/local/bin/beacon-chain
ENTRYPOINT ["beacon-chain"]
