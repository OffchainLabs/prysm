### Changed

- Pending Gloas data column queue now stores one sidecar per (block_root, column_index, peer) instead of dropping every offering after the first, so every forwarding peer of an invalid sidecar is downscored once the block arrives. Per-peer column and root caps prevent a single peer from filling the queue (consensus-specs #5199). 
