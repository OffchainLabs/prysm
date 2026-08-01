### Added

- Progressive merkleization (EIP-7688) annotations on the Gloas SSZ types.
  Generation is gated off by default, so hash tree roots are unchanged until
  `--//tools:disable_progressive_merkleization=false` (Bazel) or
  `SSZ_PROGRESSIVE=1` (`make gen`) is set.
