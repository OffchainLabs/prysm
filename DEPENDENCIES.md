# Dependency Management in Prysm

Prysm is a Go project with some cgo (C/C++) dependencies. Since the migration off Bazel,
**`go.mod` is the single source of truth** for dependencies and the build is driven by the Go
toolchain via the root `Makefile` (see `make help`).

## Go modules

The Prysm project uses standard Go modules.

### Caveat 1: Some C/C++ libraries are precompiled archives

Some of Prysm's C/C++ dependencies (notably `blst` and the herumi `bls-eth-go-binary`) ship as
precompiled, linkable archives. While there isn't necessarily anything wrong with precompiled
archives, they are a "blackbox": a third party could have compiled anything into the archive and
detecting undesired behavior would be nearly impossible. If your risk tolerance is low, compile
everything from source yourself.

A C toolchain is required (cgo). For reproducible cross-compilation, `make build platforms=all`
provisions a pinned toolchain per target (zig for linux, osxcross for darwin, mingw-w64 for
windows — see `tools/cross-toolchain/`).

### Caveat 2: Generated gRPC/protobuf and SSZ code

Generated `*.pb.go` and `*.ssz.go` files (and mocks) are committed to the tree. Regenerate them
whenever a `.proto` definition or a mocked interface changes, and commit the result:

```bash
make gen           # proto + ssz + mocks
# or a subset: make gen proto | make gen ssz | make gen mocks
```

CI enforces this via the `check-generated-go` workflow (`make gen && git diff --exit-code`).

### Caveat 3: Compile-time optimizations

Production binaries use the release build, which strips symbols, stamps version metadata, and
applies PGO for the beacon-chain:

```bash
make build mode=release            # host binaries
make build platforms=all mode=release   # all distributed targets
```

## Adding / updating dependencies

Use Go modules directly — there is no separate Bazel dependency macro to update anymore:

```bash
go get github.com/OffchainLabs/example@v1.2.3
go mod tidy
```

## Running tests

To enable conditional compilation and custom configuration for tests (more debug info, not fully
optimized), Prysm relies on Go build tags/constraints (see the official docs on
[build constraints](https://pkg.go.dev/go/build#hdr-Build_Constraints)). The `make test` target
sets these for you (the `develop` tag, plus `minimal` for the minimal pass). To run `go test`
directly, pass the `develop` tag, e.g.:

```bash
make test                                         # mainnet + minimal passes
go test ./beacon-chain/sync/initial-sync -tags develop
```
