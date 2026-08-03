### Changed

- Update consensus spec tests to `v1.7.0-alpha.13` and turn progressive SSZ
  merkleization on by default for Gloas types. The escape hatch is now
  `--disable-progressive-ssz` (and `--//tools:disable_progressive_merkleization`
  for Bazel codegen, `SSZ_PROGRESSIVE=0` for `make gen`).
