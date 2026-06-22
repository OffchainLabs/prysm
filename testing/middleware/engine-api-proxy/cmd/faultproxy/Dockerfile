# Build context must be the repo root (the binary imports the prysm module):
#   docker build -t prysm-faultproxy:local \
#     -f testing/middleware/engine-api-proxy/cmd/faultproxy/Dockerfile .
#
# The binary is installed as /json_rpc_snoop with an empty entrypoint so it is a
# drop-in for ethereum-package's snooper, whose service cmd is
# "./json_rpc_snoop -b=0.0.0.0 -p=<port> <el-engine-url>". Wire it via:
#   snooper_params: { image: prysm-faultproxy:local }
# The admin server defaults to :8552 (reachable intra-enclave by service DNS);
# do not pass --admin-addr through snooper extra_args, as Go's flag parser stops
# at the positional <el-engine-url> the snooper appends before extra_args.
FROM golang:1.26 AS build
WORKDIR /src
# Cache deps first.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /json_rpc_snoop ./testing/middleware/engine-api-proxy/cmd/faultproxy

FROM gcr.io/distroless/static-debian12
WORKDIR /
COPY --from=build /json_rpc_snoop /json_rpc_snoop
ENTRYPOINT []
CMD ["/json_rpc_snoop"]
