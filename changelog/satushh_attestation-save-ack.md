### Fixed

- Return from `SaveAttestationForPubKey` only after the flush that writes that attestation's slashing protection record, instead of after any batch flush.
