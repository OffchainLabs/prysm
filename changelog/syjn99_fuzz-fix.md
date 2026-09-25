### Ignored

- Apply findings from fuzzer in hdiff: OOB error, excessive memory allocation
- Set `base-branch` of `shogo82148/actions-go-fuzz/run` to `develop` explicitly.

### Changed

- Bound every length-prefixed collection count in hdiff decoding before allocation, so a corrupt diff returns an error instead of panicking.
- Check malformed snappy compression before decompressing via `DecodedLen`, so decoder doesn't allocate implausibly excessive memory.
