# Third Party

This directory holds local, in-tree third-party material that Prysm vendors directly
(e.g. `go-bip39`, wired in via a `replace` directive in `go.mod`).

## Modifying a dependency

Prysm builds with the Go toolchain and standard Go modules. Bazel — and its
`go_repository`-based patching of dependency sources at build time — has been removed,
so the old `third_party/*.patch` files are gone.

To change a dependency now, use one of the standard Go approaches:

- **Fork it** and point at the fork with a `replace` directive in `go.mod`:
  ```
  replace github.com/someteam/somerepo => github.com/OffchainLabs/somerepo v0.0.0-...
  ```
- **Vendor it locally** under `third_party/` and `replace` it with the local path
  (as done for `go-bip39`):
  ```
  replace github.com/tyler-smith/go-bip39 => ./third_party/go-bip39
  ```

Either way the change lives in normal Go source that `go build` compiles directly — no
separate patch step to apply.
