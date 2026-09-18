### Removed

- Removed the hidden `--disable-progressive-ssz` feature flag. Gloas (EIP-7688) mandates progressive merkleization and the generated SSZ code never honored the flag, so toggling it only produced roots matching neither the spec nor the bounded build.
- Removed the `--//tools:disable_progressive_merkleization` Bazel flag. `SSZ_PROGRESSIVE=0 make gen ssz` remains the single codegen escape hatch for the bounded merkleization form.
