### Removed

- Removed the hidden `--disable-progressive-ssz` feature flag. Gloas (EIP-7688) mandates progressive merkleization and the generated SSZ code never honored the flag, so toggling it only produced roots matching neither the spec nor the bounded build.
