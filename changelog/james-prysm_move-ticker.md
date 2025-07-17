### Fixed

- Fixes edge case starting validator client with new validator keys starts the slot ticker too early resulting in replayed slots in the main runner loop. Fixes edge case of replayed slots when waiting for account acivations. 