### Changed

- Added `proto/prysm/wrappers` holding the hash-tree-root helpers that take
  concrete proto types, so `encoding/ssz` can shed its dependency on the
  generated proto packages.
- Removed the proto-typed hash-tree-root helpers from `encoding/ssz`;
  `proto/prysm/wrappers` is now their only home and `encoding/ssz` no longer
  imports the generated proto packages.
