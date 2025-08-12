### Fixed

- Fixed integer overflow in attestation reward calculation that could occur with large validator sets and high reward multipliers. The `attestationDelta` function in Altair epoch processing now uses arbitrary precision arithmetic to prevent overflow when computing source, target, and head rewards, ensuring mathematically accurate results in all scenarios.