### Added

- The REST VC now submits `POST /eth/v2/validator/aggregate_and_proofs` with an SSZ request body, falling back to JSON when the beacon node rejects SSZ.
