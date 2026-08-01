### Changed

- Added `proto/prysm/wrappers` holding the hash-tree-root helpers that take
  concrete proto types, so `encoding/ssz` can shed its dependency on the
  generated proto packages.
