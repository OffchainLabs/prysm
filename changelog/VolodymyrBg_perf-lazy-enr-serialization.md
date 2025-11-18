## Changed

- Make ENR serialization in `compareForkENR()` lazy and log-only to avoid unnecessary allocations in the discovery hot path 
