# Build image for Anthropic's OSS Scanner (https://github.com/anthropics/oss-scanner, projects/prysm).
# The scanner builds this with the Prysm checkout as the build context, with network access, then
# audits the finished image offline. Everything the agent needs to build and test must be fetched here.
#
# Bazel is Prysm's first-class build system, but this image uses the plain Go toolchain (as the
# repository's own Dockerfile does) so that `go build` and `go test` work offline without a Bazel cache.
FROM golang:1.26.10-bookworm

ENV DEBIAN_FRONTEND=noninteractive \
    CGO_ENABLED=1 \
    CGO_CFLAGS="-D__BLST_PORTABLE__" \
    GOTOOLCHAIN=local

RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential ca-certificates curl git jq pkg-config procps \
    && rm -rf /var/lib/apt/lists/*

# scanner contract: the checkout lives inside the image, at /src
COPY . /src
WORKDIR /src

RUN go mod download

# Build every binary under cmd/ into /usr/local/bin (and dist/, where build/bazel.FindBinary looks).
RUN mkdir -p dist \
    && for bin in beacon-chain validator prysmctl client-stats; do \
         go build -o "/usr/local/bin/$bin" "./cmd/$bin" && cp "/usr/local/bin/$bin" "dist/$bin"; \
       done

# Pre-fetch the external spec-test fixtures (consensus-spec-tests, BLS and KZG vectors, network
# configs) into third_party/testdata/ so spec tests can run offline. Failure here is not fatal.
RUN go run ./tools/cmd/fetch-testdata || echo "WARNING: could not pre-fetch external test data"

# Run the maintainers' default unit-test pass (go test -tags=develop over every package except
# e2e, spec tests and minimal-preset packages). This also compiles every test binary, warming the
# build cache. A failing test or a timeout does not abort the image build.
RUN timeout -k 30s 20m go run ./build/test mainnet \
    || echo "WARNING: the mainnet unit-test pass reported failures or timed out"

# New reproducers and patches must also build with the network disabled.
ENV GOPROXY=off \
    GOSUMDB=off \
    GOFLAGS=-mod=mod

CMD ["bash"]
