### Fixed

- Send forkchoice updates to the execution client during Gloas initial-sync: notify the last imported payload when a batch fails partway, and send a plain forkchoice update from the gossip envelope path while syncing.
