### Changed
- Filtering peers for data column subnets: Added a one-epoch slack to the peer’s head slot view.
- Fetching data column sidecars: If not all requested sidecars are available for a given root, return the successfully retrieved ones along with a map indicating which could not be fetched.
- Fetching origin data column sidecars: If only some sidecars are fetched, save the retrieved ones and retry fetching the missing ones on the next attempt.

### Added
- Implemented syncing in a disjoint network with respect to data column sidecars subscribed by peers.

## Fixed
- Initial sync: Do not request data column sidecars for blocks before the retention period.