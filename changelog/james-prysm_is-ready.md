### Changed

- changed IsHealthy check to IsReady for validator client's interpretation from /eth/v1/node/health, 206 will now return false as the node is syncing. 