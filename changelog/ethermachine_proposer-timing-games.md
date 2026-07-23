### Added

- Opt-in proposer timing games support: `--enable-proposer-timing-games` and `--proposer-timing-game-delay` validator flags delay the block proposal request within the slot (clamped to stay safely before the attestation deadline), and a new `--builder-getheader-timeout` beacon node flag makes the previously hardcoded 1s builder getHeader timeout configurable.
