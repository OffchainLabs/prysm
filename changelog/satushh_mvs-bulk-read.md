### Changed

- Add a bulk `All` iterator to the multi-value slice container so a full read acquires the slice lock once instead of once per element.
- Use `All` in `ValidatorsReadOnlySeq` and `PublicKeys`, speeding up full validator-registry sweeps such as epoch precompute, committee shuffling, and registry updates.
