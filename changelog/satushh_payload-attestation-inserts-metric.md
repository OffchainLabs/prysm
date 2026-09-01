### Added

- Add `payload_attestation_inserts_total` counter, labelled by `payload_present` and `blob_data_available`, to expose the distribution of PTC claims observed by the payload-attestation pool. Counted once per PTC seat, so a validator sampled into multiple seats is weighted accordingly.
