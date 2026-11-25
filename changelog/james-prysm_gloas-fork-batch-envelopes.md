### Fixed

- Initial sync no longer stalls on a block batch that crosses the Gloas fork. The batch keeps all its blocks and fetches the execution payload envelopes for the Gloas part, instead of cutting the batch at the first Gloas block and waiting for the sync queue to reset.
