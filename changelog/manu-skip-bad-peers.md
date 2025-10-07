### Fixed 
- `findPeersWithSubnets`: If the filter function returns an error for a given peer, log an error and skip the peer instead of aborting the whole function.
- `computeIndicesByRootByPeer`: If the loop returns an error for a given peer, log an error and skip the peer instead of aborting the whole function.