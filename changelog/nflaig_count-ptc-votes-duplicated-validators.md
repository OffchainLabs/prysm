### Fixed

- Count PTC votes from duplicated validators (consensus-specs#5222): a validator sampled into the payload timeliness committee multiple times now has its vote recorded at every position it occupies, not only at `ptc.index(validator_index)`. Un-skips the corresponding `on_payload_attestation_message` fork choice spec tests.
