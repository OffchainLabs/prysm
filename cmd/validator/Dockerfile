FROM golang:1.25.1-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
COPY third_party/go-bip39 ./third_party/go-bip39
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
ARG CGO_CFLAGS="-O2 -D__BLST_PORTABLE__"
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /app ./cmd/validator

FROM gcr.io/distroless/cc-debian12:nonroot
COPY --from=builder /app /usr/local/bin/validator
ENTRYPOINT ["validator"]
