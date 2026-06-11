### Fixed

- Re-enforce `validate_on_attestation`'s same-slot index-0 rule in `OnAttestation`, so pool and pending-queue attestation replays cannot credit a same-slot payload-present vote to the full node.
