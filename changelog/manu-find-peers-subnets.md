### Fixed
- Lock the subnet mutex only when trying to find peers, so the mutex is not locked when dialing peers.