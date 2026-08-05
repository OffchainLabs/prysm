### Ignored

- Route every `c-kzg-4844` call in `beacon-chain/blockchain/kzg` through a single
  backend interface, and replace the `kzg.Bytes48`/`kzg.Bytes32` aliases of c-kzg
  types with types owned by the package.
