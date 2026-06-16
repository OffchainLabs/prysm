# Cross-compilation toolchain

This directory provides the C toolchains used to cross-compile Prysm's cgo binaries (blst,
herumi) for the five distributed run-targets:

* `linux/amd64`, `linux/arm64`
* `darwin/amd64`, `darwin/arm64`
* `windows/amd64`

It is driven by the Go build orchestrator (`build/cross`, invoked by `make build platforms=all`),
which picks a C compiler per OS and auto-provisions it via the idempotent `install-*.sh` scripts
here (no manual setup):

| Target | Compiler | Provisioned by | Host requirement |
|--------|----------|----------------|------------------|
| linux | `zig cc` (glibc 2.31 baseline) | `install-zig.sh` | any host |
| darwin | osxcross `o64-clang` / `oa64-clang` (MacOSX12.3 SDK) | `install-osxcross.sh` (→ `install_osxcross.sh`, `link_osxcross.sh`) | linux/amd64 |
| windows | mingw-w64 (`x86_64-w64-mingw32-gcc`) | `install-mingw.sh` | linux/amd64 |

## Usage

```bash
make build platforms=all              # all five targets → dist/
make build platforms=all mode=release # stamped + stripped + PGO'd
```

The linux targets use zig and build from any host (so `make build docker=true`, which builds the
linux images, also runs on macOS). The darwin (osxcross) and windows (mingw-w64) targets require a
linux/amd64 host — exactly as the old Bazel RBE worker did. For CI, bake osxcross + mingw-w64 into
the build image to avoid the per-run install.

See `BAZEL_MIGRATION.md` (Phase 4) for the full rationale and per-target toolchain notes.
