### Fixed

- Source the gRPC validator event stream (`StreamSlots` with `VerifiedOnly`) from new-head events instead of block-processed events, matching the REST API's head topic semantics.
