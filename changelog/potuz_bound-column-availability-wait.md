### Fixed

- Bound the data-column availability wait during block-batch (initial) sync. `areDataColumnsAvailable` waited indefinitely for a block's custody columns (its slot-end case only logs), so a single block whose columns were momentarily unavailable could block the sequential batch-import pipeline forever and stall sync at a batch boundary. The wait is now bounded to one slot in the sync path only (the head/gossip and execution-payload-envelope paths are unchanged); on timeout the batch errors and is retried, which re-fetches the columns.
