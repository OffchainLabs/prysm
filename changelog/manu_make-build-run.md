### Added

- `make build [<bin>...] [flags=...]`: Build Prysm binaries without Bazel.
- `make run <bin> [flags=...] [-- <args>]`: Build and run a Prysm binary without Bazel.

### Changed

- `make gen`: Cache generation inputs to skip re-generating files that are already up to date.
