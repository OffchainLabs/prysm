### Fixed

- Verify the parent execution payload envelope of the first Gloas block in an initial-sync batch, and the availability of that payload's data columns, before importing the batch.
- Recover a parent execution payload envelope that is missing from a range response from peers by root, with a by-range fallback, instead of failing the batch.
- Prevent oversized payload requests from stalling initial sync.
- Limit parent payload recovery attempts and penalize invalid responses without penalizing legitimate empty or alternate-fork range responses.
