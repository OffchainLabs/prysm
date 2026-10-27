### Fixed

- Fork choice no longer counts Gloas payload-present attestations whose execution payload is unknown, matching the `is_payload_verified` check in `validate_on_attestation` (https://github.com/ethereum/consensus-specs/pull/4918).
