### Added

- `tools/genception`, a `GOPACKAGESDRIVER` that answers `go/packages` queries
  from a Bazel-supplied package inventory, so code generators that load types
  through `go/packages` can run inside the Bazel sandbox.
