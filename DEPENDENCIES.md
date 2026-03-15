# Dependency Management in Prysm

Prysm is a Go project with many complicated dependencies, including some C++ based libraries.
Dependencies are managed via Go modules (`go.mod` / `go.sum`).

## Go Module support

The Prysm project uses standard Go modules for dependency management.

### Caveat 1: Some C++ libraries are precompiled archives

Given some of Prysm's C++ dependencies have very complicated project structures which make building
difficult or impossible with `go build` alone. Additionally, building C++ dependencies with certain
compilers, like clang / LLVM, offer a significant performance improvement. To get around this
issue, C++ dependencies have been precompiled as linkable archives. While there isn't necessarily
anything bad about precompiled archives, these files are a "blackbox" which a 3rd party author
could have compiled anything for this archive and detecting undesired behavior would be nearly
impossible. If your risk tolerance is low, always compile everything from source yourself,
including complicated C++ dependencies.

### Caveat 2: Generated gRPC protobuf libraries

Generated pb.go files should be regenerated when protobuf definitions change. Run the following
scripts to ensure generated files are up to date. Furthermore, Prysm generates SSZ marshal related
code based on defined data structures. These generated files must also be updated and checked in
as frequently.

```bash
./hack/update-go-pbs.sh
./hack/update-go-ssz.sh
```

## Adding / updating dependencies

Add your dependency as you would with Go modules:

```bash
go get github.com/OffchainLabs/example@v1.2.3
go mod tidy
```

## Running tests

To enable conditional compilation and custom configuration for tests (where compiled code has more
debug info, while not being completely optimized), we rely on Go's build tags/constraints mechanism
(see official docs on [build constraints](https://pkg.go.dev/go/build#hdr-Build_Constraints)).
Therefore, whenever using `go test`, do not forget to pass in extra build tag, e.g.:

```bash
go test ./beacon-chain/sync/initial-sync -tags develop
```

### Test data setup

Spec tests and config param tests require external data that is not checked in.
Download it before running tests:

```bash
# Spec test vectors + consensus configs + testnet configs
./hack/download-spec-tests.sh

# E2E test binaries (lighthouse, web3signer)
./hack/download-e2e-deps.sh
```

See `testing/spectest/README.md` for more details.
