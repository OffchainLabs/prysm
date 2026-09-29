### Ignored

- Added a `fake_crypto` build tag that swaps the `crypto/bls` backend for one that accepts the consensus spec's stub signature (`0x11*96`) and treats every signature as valid, so `bls_setting: 2` spec test vectors can be run.
- Added central `bls_setting` filtering to the spectest harness, plus `make test <spectest kind> bls=fake` and CI jobs that run the spec tests under the fake backend. 